package main

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,50}$`)

// findUserByParam ดึง user ตาม :id ใน URL ถ้าไม่เจอหรือ error จะตอบกลับให้เองแล้วคืน false
func findUserByParam(c *gin.Context) (User, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return User{}, false
	}

	var u User
	err = db.First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return User{}, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot get user"})
		return User{}, false
	}
	return u, true
}

// 1. Create — POST /api/auth/register
func register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Keycloak เก็บ username เป็นตัวพิมพ์เล็กเสมอ จึงทำให้ตรงกันตั้งแต่ตรงนี้
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !usernamePattern.MatchString(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username may contain only a-z, 0-9, dot, underscore and dash"})
		return
	}

	ctx := c.Request.Context()

	// ขั้นที่ 1: สร้างบัญชี (พร้อมรหัสผ่าน) ใน Keycloak
	keycloakID, err := createKeycloakUser(ctx, req)
	if isKeycloakConflict(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
		return
	}
	if err != nil {
		log.Printf("register: create keycloak user: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot create account in identity provider"})
		return
	}

	// ขั้นที่ 2: ให้ role USER
	if err := setKeycloakRole(ctx, keycloakID, RoleUser); err != nil {
		log.Printf("register: assign role: %v", err)
		deleteKeycloakUser(keycloakID)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot assign role in identity provider"})
		return
	}

	// ขั้นที่ 3: บันทึกโปรไฟล์ลง auth_db ถ้าไม่สำเร็จให้ลบบัญชีใน Keycloak ทิ้ง
	u := User{
		KeycloakID: keycloakID,
		Username:   req.Username,
		Email:      req.Email,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		Role:       RoleUser,
		Status:     StatusActive,
	}
	if err := db.Create(&u).Error; err != nil {
		deleteKeycloakUser(keycloakID)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			c.JSON(http.StatusConflict, gin.H{"error": "username or email already exists"})
			return
		}
		log.Printf("register: insert user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot create user"})
		return
	}

	c.JSON(http.StatusCreated, u)
}

// 2. List — GET /api/users
func listUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	role := strings.ToUpper(c.Query("role"))
	status := strings.ToUpper(c.Query("status"))

	if role != "" && !validRoles[role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be USER, STAFF or ADMIN"})
		return
	}
	if status != "" && !validStatuses[status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be ACTIVE or DEACTIVATED"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// ค่อย ๆ เพิ่มเงื่อนไขเฉพาะตัวกรองที่ส่งมา (GORM ใส่ค่าแทน ? ให้เอง ปลอดภัยจาก SQL Injection)
	query := db.Model(&User{})
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("(username ILIKE ? OR email ILIKE ? OR first_name ILIKE ? OR last_name ILIKE ?)",
			like, like, like, like)
	}
	if role != "" {
		query = query.Where("role = ?", role)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	// Session ทำให้ใช้ query เดิมซ้ำได้ทั้งตอนนับและตอนดึงข้อมูล
	query = query.Session(&gorm.Session{})

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot count users"})
		return
	}

	users := []User{}
	if err := query.Order("id").Limit(limit).Offset((page - 1) * limit).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot list users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  users,
		"page":  page,
		"limit": limit,
		"total": total,
	})
}

// 3. Read own — GET /api/users/me
func getMe(c *gin.Context) {
	claims := currentClaims(c)

	var u User
	err := db.Where("keycloak_id = ?", claims.Subject).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user profile not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot get user"})
		return
	}
	c.JSON(http.StatusOK, u)
}

// GET /api/users/:id — สำหรับ Admin/Staff และให้ service อื่นตรวจว่ามี user นี้จริง
func getUser(c *gin.Context) {
	u, ok := findUserByParam(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, u)
}

// 4. Update — PUT /api/users/:id
func updateUser(c *gin.Context) {
	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	u, ok := findUserByParam(c)
	if !ok {
		return
	}

	// แก้ที่ Keycloak ด้วย ข้อมูลใน token (เช่น email) จะได้ตรงกับในฐานข้อมูล
	err := updateKeycloakUser(c.Request.Context(), u.KeycloakID, req)
	if isKeycloakConflict(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}
	if err != nil {
		log.Printf("update user %d: keycloak: %v", u.ID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot update account in identity provider"})
		return
	}

	u.Email = req.Email
	u.FirstName = req.FirstName
	u.LastName = req.LastName
	err = db.Save(&u).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}
	if err != nil {
		log.Printf("update user %d: %v", u.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot update user"})
		return
	}

	c.JSON(http.StatusOK, u)
}

// 5. Delete/Cancel — PUT /api/users/:id/deactivate
func deactivateUser(c *gin.Context) {
	u, ok := findUserByParam(c)
	if !ok {
		return
	}

	if u.KeycloakID == currentClaims(c).Subject {
		c.JSON(http.StatusConflict, gin.H{"error": "you cannot deactivate your own account"})
		return
	}
	if u.Status == StatusDeactivated {
		c.JSON(http.StatusConflict, gin.H{"error": "user is already deactivated"})
		return
	}

	if err := disableKeycloakUser(c.Request.Context(), u.KeycloakID); err != nil {
		log.Printf("deactivate user %d: keycloak: %v", u.ID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot disable account in identity provider"})
		return
	}

	u.Status = StatusDeactivated
	if err := db.Save(&u).Error; err != nil {
		log.Printf("deactivate user %d: %v", u.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot deactivate user"})
		return
	}

	c.JSON(http.StatusOK, u)
}

// 6. Special — PUT /api/users/:id/role
func changeRole(c *gin.Context) {
	var req ChangeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be USER, STAFF or ADMIN"})
		return
	}

	u, ok := findUserByParam(c)
	if !ok {
		return
	}

	// กัน Admin ลด role ตัวเองจนไม่เหลือ Admin ในระบบ
	if u.KeycloakID == currentClaims(c).Subject {
		c.JSON(http.StatusConflict, gin.H{"error": "you cannot change your own role"})
		return
	}
	if u.Status == StatusDeactivated {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot change role of a deactivated user"})
		return
	}
	if u.Role == req.Role {
		c.JSON(http.StatusOK, u)
		return
	}

	// role ใน token มาจาก Keycloak จึงต้องเปลี่ยนที่ Keycloak ด้วย service อื่นถึงจะเห็น role ใหม่
	if err := setKeycloakRole(c.Request.Context(), u.KeycloakID, req.Role); err != nil {
		log.Printf("change role of user %d: keycloak: %v", u.ID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot change role in identity provider"})
		return
	}

	u.Role = req.Role
	if err := db.Save(&u).Error; err != nil {
		log.Printf("change role of user %d: %v", u.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot change role"})
		return
	}

	c.JSON(http.StatusOK, u)
}

// 7. Report — GET /api/reports/users/summary
func userSummary(c *gin.Context) {
	sum := UserSummary{
		ByRole:   map[string]int64{RoleUser: 0, RoleStaff: 0, RoleAdmin: 0},
		ByStatus: map[string]int64{StatusActive: 0, StatusDeactivated: 0},
	}

	type groupCount struct {
		Name  string
		Count int64
	}
	var byRole, byStatus []groupCount

	err := db.Model(&User{}).Count(&sum.Total).Error
	if err == nil {
		err = db.Model(&User{}).Select("role AS name, COUNT(*) AS count").Group("role").Scan(&byRole).Error
	}
	if err == nil {
		err = db.Model(&User{}).Select("status AS name, COUNT(*) AS count").Group("status").Scan(&byStatus).Error
	}
	if err == nil {
		err = db.Model(&User{}).Where("created_at >= NOW() - INTERVAL '30 days'").Count(&sum.NewLast30Days).Error
	}
	if err != nil {
		log.Printf("user summary: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot build user summary"})
		return
	}

	for _, r := range byRole {
		sum.ByRole[r.Name] = r.Count
	}
	for _, s := range byStatus {
		sum.ByStatus[s.Name] = s.Count
	}
	c.JSON(http.StatusOK, sum)
}
