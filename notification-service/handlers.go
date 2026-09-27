package main

import (
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// currentUserID หา users.id ของคนที่ login จาก auth-service ถ้าไม่สำเร็จจะตอบกลับให้เองแล้วคืน false
func currentUserID(c *gin.Context) (int64, bool) {
	u, err := getMyAuthUser(c.Request.Context(), c.GetHeader("Authorization"))
	switch {
	case err == nil:
		return u.ID, true
	case errors.Is(err, errAuthUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
	case errors.Is(err, errAuthNotFound):
		c.JSON(http.StatusForbidden, gin.H{"error": "user profile not found"})
	default:
		log.Printf("get current user from auth-service: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot reach auth-service"})
	}
	return 0, false
}

// findOwnNotification ดึง notification ตาม :id และตรวจว่าเป็นของคนที่ login
// ถ้า id ผิด / ไม่เจอ / เป็นของคนอื่น จะตอบกลับให้เองแล้วคืน false
func findOwnNotification(c *gin.Context) (Notification, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return Notification{}, false
	}

	userID, ok := currentUserID(c)
	if !ok {
		return Notification{}, false
	}

	var n Notification
	err = db.First(&n, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
		return Notification{}, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot get notification"})
		return Notification{}, false
	}

	// ทุก role (รวม ADMIN) เข้าถึงได้เฉพาะ notification ของตัวเอง
	if n.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "you can only access your own notifications"})
		return Notification{}, false
	}
	return n, true
}

// 1. Create — POST /api/notifications (STAFF, ADMIN)
func createNotification(c *gin.Context) {
	var req CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Message = strings.TrimSpace(req.Message)
	if req.Title == "" || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title and message must not be blank"})
		return
	}
	if req.Type == "" {
		req.Type = TypeGeneral
	}

	// ตรวจว่ามี user นี้จริงใน auth-service (ใช้ token ของ STAFF/ADMIN ที่เรียกมา)
	_, err := getAuthUserByID(c.Request.Context(), req.UserID, c.GetHeader("Authorization"))
	switch {
	case err == nil:
	case errors.Is(err, errAuthNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "user not found"})
		return
	case errors.Is(err, errAuthUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	case errors.Is(err, errAuthForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
		return
	default:
		log.Printf("create notification: check user %d: %v", req.UserID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "cannot reach auth-service"})
		return
	}

	n := Notification{
		UserID:  req.UserID,
		Title:   req.Title,
		Message: req.Message,
		Type:    req.Type,
	}
	if err := db.Create(&n).Error; err != nil {
		log.Printf("create notification: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot create notification"})
		return
	}

	c.JSON(http.StatusCreated, n)
}

// 2. List — GET /api/notifications?is_read=&type=&page=&limit= (เฉพาะของตัวเอง)
func listNotifications(c *gin.Context) {
	notifType := strings.ToUpper(c.Query("type"))
	if notifType != "" && !validTypes[notifType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be one of " + strings.Join(allTypes, ", ")})
		return
	}

	var isRead *bool
	if v := c.Query("is_read"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "is_read must be true or false"})
			return
		}
		isRead = &b
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// ค่อย ๆ เพิ่มเงื่อนไขเฉพาะตัวกรองที่ส่งมา โดยเริ่มจาก user_id ของตัวเองเสมอ
	query := db.Model(&Notification{}).Where("user_id = ?", userID)
	if isRead != nil {
		query = query.Where("is_read = ?", *isRead)
	}
	if notifType != "" {
		query = query.Where("type = ?", notifType)
	}
	// Session ทำให้ใช้ query เดิมซ้ำได้ทั้งตอนนับและตอนดึงข้อมูล
	query = query.Session(&gorm.Session{})

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot count notifications"})
		return
	}

	notifications := []Notification{}
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Offset((page - 1) * limit).Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot list notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  notifications,
		"page":  page,
		"limit": limit,
		"total": total,
	})
}

// 3. Read one — GET /api/notifications/:id (เจ้าของเท่านั้น)
func getNotification(c *gin.Context) {
	n, ok := findOwnNotification(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, n)
}

// 4. Update — PUT /api/notifications/:id/read (เจ้าของเท่านั้น)
func markAsRead(c *gin.Context) {
	n, ok := findOwnNotification(c)
	if !ok {
		return
	}
	if !n.IsRead {
		if err := db.Model(&n).Update("is_read", true).Error; err != nil {
			log.Printf("mark notification %d as read: %v", n.ID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot update notification"})
			return
		}
	}
	c.JSON(http.StatusOK, n)
}

// 5. Delete — DELETE /api/notifications/:id (เจ้าของเท่านั้น)
func deleteNotification(c *gin.Context) {
	n, ok := findOwnNotification(c)
	if !ok {
		return
	}
	if err := db.Delete(&n).Error; err != nil {
		log.Printf("delete notification %d: %v", n.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot delete notification"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "notification deleted"})
}

// 7. Report — GET /api/reports/notifications/summary (ADMIN)
func notificationSummary(c *gin.Context) {
	sum := NotificationSummary{ByType: map[string]int64{}}
	for _, t := range allTypes {
		sum.ByType[t] = 0
	}

	type groupCount struct {
		Type   string
		IsRead bool
		Count  int64
	}
	var rows []groupCount
	err := db.Model(&Notification{}).Select("type, is_read, COUNT(*) AS count").Group("type, is_read").Scan(&rows).Error
	if err != nil {
		log.Printf("notification summary: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot build notification summary"})
		return
	}

	for _, r := range rows {
		sum.Total += r.Count
		if r.IsRead {
			sum.Read += r.Count
		}
		sum.ByType[r.Type] += r.Count
	}
	sum.Unread = sum.Total - sum.Read
	if sum.Total > 0 {
		sum.ReadRate = math.Round(float64(sum.Read)/float64(sum.Total)*10000) / 100
	}
	c.JSON(http.StatusOK, sum)
}
