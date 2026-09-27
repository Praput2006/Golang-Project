package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// test ในไฟล์นี้รันได้ทันทีโดยไม่ต้องเปิด Docker:
// - ฐานข้อมูลใช้ SQLite ในหน่วยความจำแทน PostgreSQL (สร้างใหม่ทุก test)
// - auth-service ใช้ server ปลอม (httptest) ที่ตอบ /api/users/me และ /api/users/:id
// - token เซ็นด้วย RSA key ปลอมแบบเดียวกับ Keycloak

const (
	testIssuer      = "http://keycloak.test/realms/unibook"
	testInternalKey = "test-internal-key"
)

// testKey ใช้แทน private key ของ Keycloak สำหรับเซ็น token ปลอมใน test
var testKey, _ = rsa.GenerateKey(rand.Reader, 2048)

func testKeyfunc(*jwt.Token) (any, error) {
	return &testKey.PublicKey, nil
}

// ผู้ใช้ทดสอบ: keycloak_id (sub ใน token) → users.id และ role
type testUser struct {
	sub  string
	id   int64
	role string
}

var (
	adminUser = testUser{"kc-admin", 1, RoleAdmin}
	user1     = testUser{"kc-user1", 2, RoleUser}
	user2     = testUser{"kc-user2", 3, RoleUser}
	staffUser = testUser{"kc-staff", 4, RoleStaff}
	testUsers = []testUser{adminUser, user1, user2, staffUser}
)

// makeToken สร้าง access token ปลอมแบบเดียวกับที่ Keycloak ออกให้ user คนนี้
func makeToken(t *testing.T, u testUser) string {
	t.Helper()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.sub,
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		RealmAccess: RealmAccess{Roles: []string{u.role}},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

// newFakeAuthService จำลอง auth-service: /api/users/me ดู sub จาก token, /api/users/:id ดูว่ามี user จริงไหม
func newFakeAuthService(t *testing.T) *httptest.Server {
	t.Helper()
	bySub := map[string]testUser{}
	byID := map[string]testUser{}
	for _, u := range testUsers {
		bySub[u.sub] = u
		byID[fmt.Sprint(u.id)] = u
	}

	writeUser := func(w http.ResponseWriter, u testUser) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": u.id, "keycloak_id": u.sub, "role": u.role, "status": "ACTIVE"})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/me", func(w http.ResponseWriter, r *http.Request) {
		tokenString, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims := &Claims{}
		if _, _, err := jwt.NewParser().ParseUnverified(tokenString, claims); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		u, ok := bySub[claims.Subject]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeUser(w, u)
	})
	mux.HandleFunc("GET /api/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		u, ok := byID[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeUser(w, u)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTestRouter เตรียมฐานข้อมูลใหม่ + auth-service ปลอม แล้วคืน router ตัวจริงของ service
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, _ := testDB.DB()
	sqlDB.SetMaxOpenConns(1) // :memory: แยกฐานข้อมูลต่อ connection จึงใช้ connection เดียว
	if err := testDB.AutoMigrate(&Notification{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db = testDB

	connectAuthService(newFakeAuthService(t).URL)

	return setupRouter(AuthMiddleware(testKeyfunc, testIssuer), InternalKeyMiddleware(testInternalKey))
}

func sendRequest(r http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sendInternal(r http.Handler, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return v
}

func seedNotification(t *testing.T, userID int64, notifType string, isRead bool) Notification {
	t.Helper()
	n := Notification{UserID: userID, Title: "seed", Message: "seed message", Type: notifType, IsRead: isRead}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return n
}

func countNotifications(t *testing.T) int64 {
	t.Helper()
	var n int64
	db.Model(&Notification{}).Count(&n)
	return n
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, want, w.Body.String())
	}
}

// ---------- 1. Create ----------

func TestCreateNotification_Success(t *testing.T) {
	r := newTestRouter(t)
	body := fmt.Sprintf(`{"user_id": %d, "title": "ห้อง 301 ปิดปรับปรุง", "message": "ปิดวันศุกร์นี้"}`, user1.id)

	for _, creator := range []testUser{staffUser, adminUser} {
		t.Run(creator.role, func(t *testing.T) {
			w := sendRequest(r, http.MethodPost, "/api/notifications", makeToken(t, creator), body)
			expectStatus(t, w, http.StatusCreated)

			n := decode[Notification](t, w)
			if n.ID == 0 || n.UserID != user1.id || n.Type != TypeGeneral || n.IsRead {
				t.Errorf("unexpected notification: %+v", n)
			}
		})
	}
	if got := countNotifications(t); got != 2 {
		t.Errorf("notifications in db = %d, want 2", got)
	}
}

func TestCreateNotification_Unauthorized(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodPost, "/api/notifications", "", `{"user_id": 2, "title": "x", "message": "y"}`)
	expectStatus(t, w, http.StatusUnauthorized)
}

func TestCreateNotification_ForbiddenForUser(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodPost, "/api/notifications", makeToken(t, user1), `{"user_id": 2, "title": "x", "message": "y"}`)
	expectStatus(t, w, http.StatusForbidden)
	if got := countNotifications(t); got != 0 {
		t.Errorf("notifications in db = %d, want 0", got)
	}
}

func TestCreateNotification_BadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid json", `{bad`},
		{"missing user_id", `{"title": "x", "message": "y"}`},
		{"missing title", `{"user_id": 2, "message": "y"}`},
		{"blank title", `{"user_id": 2, "title": "   ", "message": "y"}`},
		{"missing message", `{"user_id": 2, "title": "x"}`},
		{"title too long", `{"user_id": 2, "title": "` + strings.Repeat("ก", 101) + `", "message": "y"}`},
		{"invalid type", `{"user_id": 2, "title": "x", "message": "y", "type": "XYZ"}`},
		{"user not found in auth-service", `{"user_id": 999, "title": "x", "message": "y"}`},
	}

	r := newTestRouter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sendRequest(r, http.MethodPost, "/api/notifications", makeToken(t, staffUser), tt.body)
			expectStatus(t, w, http.StatusBadRequest)
		})
	}
	if got := countNotifications(t); got != 0 {
		t.Errorf("notifications in db = %d, want 0", got)
	}
}

func TestCreateNotification_AuthServiceDown(t *testing.T) {
	r := newTestRouter(t)
	connectAuthService("http://127.0.0.1:1") // ไม่มีอะไรฟังอยู่

	w := sendRequest(r, http.MethodPost, "/api/notifications", makeToken(t, staffUser), `{"user_id": 2, "title": "x", "message": "y"}`)
	expectStatus(t, w, http.StatusBadGateway)
}

// ---------- 2. List ----------

type listResponse struct {
	Data  []Notification `json:"data"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
	Total int64          `json:"total"`
}

func TestListNotifications_OnlyOwn(t *testing.T) {
	r := newTestRouter(t)
	seedNotification(t, user1.id, TypeGeneral, false)
	seedNotification(t, user1.id, TypeBookingApproved, true)
	other := seedNotification(t, user2.id, TypeGeneral, false)

	w := sendRequest(r, http.MethodGet, "/api/notifications", makeToken(t, user1), "")
	expectStatus(t, w, http.StatusOK)

	resp := decode[listResponse](t, w)
	if resp.Total != 2 || len(resp.Data) != 2 {
		t.Fatalf("total = %d, len = %d, want 2", resp.Total, len(resp.Data))
	}
	for _, n := range resp.Data {
		if n.UserID != user1.id || n.ID == other.ID {
			t.Errorf("user1 can see notification of another user: %+v", n)
		}
	}
}

func TestListNotifications_FilterAndPagination(t *testing.T) {
	r := newTestRouter(t)
	for i := 0; i < 3; i++ {
		seedNotification(t, user1.id, TypeGeneral, false)
	}
	seedNotification(t, user1.id, TypeGeneral, true)
	seedNotification(t, user1.id, TypeBookingRejected, false)

	tests := []struct {
		query     string
		wantTotal int64
		wantLen   int
	}{
		{"?is_read=false", 4, 4},
		{"?is_read=true", 1, 1},
		{"?type=BOOKING_REJECTED", 1, 1},
		{"?type=GENERAL&is_read=false", 3, 3},
		{"?limit=2&page=1", 5, 2},
		{"?limit=2&page=3", 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, "/api/notifications"+tt.query, makeToken(t, user1), "")
			expectStatus(t, w, http.StatusOK)
			resp := decode[listResponse](t, w)
			if resp.Total != tt.wantTotal || len(resp.Data) != tt.wantLen {
				t.Errorf("total = %d, len = %d, want %d, %d", resp.Total, len(resp.Data), tt.wantTotal, tt.wantLen)
			}
		})
	}
}

func TestListNotifications_BadRequest(t *testing.T) {
	r := newTestRouter(t)
	for _, q := range []string{"?is_read=abc", "?type=XYZ"} {
		t.Run(q, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, "/api/notifications"+q, makeToken(t, user1), "")
			expectStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestListNotifications_Unauthorized(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodGet, "/api/notifications", "", "")
	expectStatus(t, w, http.StatusUnauthorized)
}

// ---------- 3. Read one ----------

func TestGetNotification_Success(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)

	w := sendRequest(r, http.MethodGet, fmt.Sprintf("/api/notifications/%d", n.ID), makeToken(t, user1), "")
	expectStatus(t, w, http.StatusOK)
	if got := decode[Notification](t, w); got.ID != n.ID {
		t.Errorf("id = %d, want %d", got.ID, n.ID)
	}
}

func TestGetNotification_OtherUserForbidden(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)

	// ADMIN ก็ดูของคนอื่นไม่ได้ (ของตัวเองเท่านั้น)
	for _, u := range []testUser{user2, adminUser} {
		t.Run(u.sub, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, fmt.Sprintf("/api/notifications/%d", n.ID), makeToken(t, u), "")
			expectStatus(t, w, http.StatusForbidden)
		})
	}
}

func TestGetNotification_NotFound(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodGet, "/api/notifications/999", makeToken(t, user1), "")
	expectStatus(t, w, http.StatusNotFound)
}

func TestNotificationByID_InvalidID(t *testing.T) {
	r := newTestRouter(t)
	for _, id := range []string{"abc", "0", "-1", "1.5"} {
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			t.Run(method+" "+id, func(t *testing.T) {
				w := sendRequest(r, method, "/api/notifications/"+id, makeToken(t, user1), "")
				expectStatus(t, w, http.StatusBadRequest)
			})
		}
		t.Run("PUT "+id, func(t *testing.T) {
			w := sendRequest(r, http.MethodPut, "/api/notifications/"+id+"/read", makeToken(t, user1), "")
			expectStatus(t, w, http.StatusBadRequest)
		})
	}
}

// ---------- 4. Mark as read ----------

func TestMarkAsRead_Success(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)
	path := fmt.Sprintf("/api/notifications/%d/read", n.ID)

	// กดซ้ำได้ ผลเหมือนเดิม
	for i := 0; i < 2; i++ {
		w := sendRequest(r, http.MethodPut, path, makeToken(t, user1), "")
		expectStatus(t, w, http.StatusOK)
		if got := decode[Notification](t, w); !got.IsRead {
			t.Errorf("response is_read = false, want true")
		}
	}

	var stored Notification
	db.First(&stored, n.ID)
	if !stored.IsRead {
		t.Errorf("db is_read = false, want true")
	}
}

func TestMarkAsRead_OtherUserForbidden(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)

	w := sendRequest(r, http.MethodPut, fmt.Sprintf("/api/notifications/%d/read", n.ID), makeToken(t, user2), "")
	expectStatus(t, w, http.StatusForbidden)

	var stored Notification
	db.First(&stored, n.ID)
	if stored.IsRead {
		t.Errorf("user2 marked user1's notification as read")
	}
}

func TestMarkAsRead_NotFound(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodPut, "/api/notifications/999/read", makeToken(t, user1), "")
	expectStatus(t, w, http.StatusNotFound)
}

// ---------- 5. Delete ----------

func TestDeleteNotification_Success(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)
	path := fmt.Sprintf("/api/notifications/%d", n.ID)

	w := sendRequest(r, http.MethodDelete, path, makeToken(t, user1), "")
	expectStatus(t, w, http.StatusOK)
	if got := countNotifications(t); got != 0 {
		t.Errorf("notifications in db = %d, want 0", got)
	}

	// ลบซ้ำ → ไม่มีแล้ว
	w = sendRequest(r, http.MethodDelete, path, makeToken(t, user1), "")
	expectStatus(t, w, http.StatusNotFound)
}

func TestDeleteNotification_OtherUserForbidden(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)

	w := sendRequest(r, http.MethodDelete, fmt.Sprintf("/api/notifications/%d", n.ID), makeToken(t, user2), "")
	expectStatus(t, w, http.StatusForbidden)
	if got := countNotifications(t); got != 1 {
		t.Errorf("user2 deleted user1's notification")
	}
}

func TestDeleteNotification_Unauthorized(t *testing.T) {
	r := newTestRouter(t)
	n := seedNotification(t, user1.id, TypeGeneral, false)

	w := sendRequest(r, http.MethodDelete, fmt.Sprintf("/api/notifications/%d", n.ID), "", "")
	expectStatus(t, w, http.StatusUnauthorized)
}

// ---------- 6. Event จาก Booking Service ----------

func TestBookingEvent_CreatesNotification(t *testing.T) {
	tests := []struct {
		event        string
		wantType     string
		wantTitle    string
		wantInMsg    string
		rejectReason string
	}{
		{EventBookingCreated, TypeBookingCreated, "สร้างการจองสำเร็จ", "รอการอนุมัติ", ""},
		{EventBookingApproved, TypeBookingApproved, "การจองได้รับการอนุมัติ", "ได้รับการอนุมัติแล้ว", ""},
		{EventBookingRejected, TypeBookingRejected, "การจองถูกปฏิเสธ", "เหตุผล: ห้องปิดปรับปรุง", "ห้องปิดปรับปรุง"},
	}

	r := newTestRouter(t)
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			body := fmt.Sprintf(`{"event": %q, "booking_id": 12, "user_id": %d, "facility_name": "ห้อง 301",
				"date": "2026-10-01", "start_time": "09:00", "end_time": "11:00", "reject_reason": %q}`,
				tt.event, user1.id, tt.rejectReason)
			w := sendInternal(r, "/internal/events", testInternalKey, body)
			expectStatus(t, w, http.StatusCreated)

			n := decode[Notification](t, w)
			if n.UserID != user1.id || n.Type != tt.wantType || n.Title != tt.wantTitle {
				t.Errorf("unexpected notification: %+v", n)
			}
			for _, part := range []string{"การจอง #12", "ห้อง 301", "วันที่ 2026-10-01", "เวลา 09:00-11:00", tt.wantInMsg} {
				if !strings.Contains(n.Message, part) {
					t.Errorf("message %q does not contain %q", n.Message, part)
				}
			}
		})
	}

	// เจ้าของการจองเห็น notification ทั้ง 3 รายการผ่าน API ปกติ
	w := sendRequest(r, http.MethodGet, "/api/notifications", makeToken(t, user1), "")
	if resp := decode[listResponse](t, w); resp.Total != 3 {
		t.Errorf("user1 total = %d, want 3", resp.Total)
	}
}

func TestBookingEvent_InvalidKey(t *testing.T) {
	r := newTestRouter(t)
	body := `{"event": "booking.created", "booking_id": 12, "user_id": 2}`

	for _, key := range []string{"", "wrong-key"} {
		w := sendInternal(r, "/internal/events", key, body)
		expectStatus(t, w, http.StatusUnauthorized)
	}

	// token ของ ADMIN ใช้แทน internal key ไม่ได้
	w := sendRequest(r, http.MethodPost, "/internal/events", makeToken(t, adminUser), body)
	expectStatus(t, w, http.StatusUnauthorized)

	if got := countNotifications(t); got != 0 {
		t.Errorf("notifications in db = %d, want 0", got)
	}
}

func TestBookingEvent_BadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unsupported event", `{"event": "booking.cancelled", "booking_id": 12, "user_id": 2}`},
		{"missing booking_id", `{"event": "booking.created", "user_id": 2}`},
		{"missing user_id", `{"event": "booking.created", "booking_id": 12}`},
		{"invalid json", `{bad`},
	}

	r := newTestRouter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sendInternal(r, "/internal/events", testInternalKey, tt.body)
			expectStatus(t, w, http.StatusBadRequest)
		})
	}
}

func TestDescribeBooking(t *testing.T) {
	tests := []struct {
		name string
		in   BookingEventRequest
		want string
	}{
		{"id only", BookingEventRequest{BookingID: 5}, "การจอง #5"},
		{"facility id fallback", BookingEventRequest{BookingID: 5, FacilityID: 3}, "การจอง #5 (ห้อง/พื้นที่ #3)"},
		{"full", BookingEventRequest{BookingID: 5, FacilityID: 3, FacilityName: "ห้อง 301", Date: "2026-10-01", StartTime: "09:00", EndTime: "11:00"},
			"การจอง #5 (ห้อง 301 วันที่ 2026-10-01 เวลา 09:00-11:00)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeBooking(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------- 7. Report ----------

func TestNotificationSummary_Admin(t *testing.T) {
	r := newTestRouter(t)
	seedNotification(t, user1.id, TypeGeneral, true)
	seedNotification(t, user1.id, TypeBookingCreated, false)
	seedNotification(t, user2.id, TypeBookingCreated, true)
	seedNotification(t, user2.id, TypeBookingRejected, false)

	w := sendRequest(r, http.MethodGet, "/api/reports/notifications/summary", makeToken(t, adminUser), "")
	expectStatus(t, w, http.StatusOK)

	sum := decode[NotificationSummary](t, w)
	if sum.Total != 4 || sum.Read != 2 || sum.Unread != 2 || sum.ReadRate != 50 {
		t.Errorf("unexpected totals: %+v", sum)
	}
	wantByType := map[string]int64{
		TypeGeneral: 1, TypeBookingCreated: 2, TypeBookingApproved: 0, TypeBookingRejected: 1, TypeBookingCancelled: 0,
	}
	for k, v := range wantByType {
		if sum.ByType[k] != v {
			t.Errorf("by_type[%s] = %d, want %d", k, sum.ByType[k], v)
		}
	}
}

func TestNotificationSummary_Empty(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodGet, "/api/reports/notifications/summary", makeToken(t, adminUser), "")
	expectStatus(t, w, http.StatusOK)

	sum := decode[NotificationSummary](t, w)
	if sum.Total != 0 || sum.ReadRate != 0 || len(sum.ByType) != len(allTypes) {
		t.Errorf("unexpected empty summary: %+v", sum)
	}
}

func TestNotificationSummary_NonAdminForbidden(t *testing.T) {
	r := newTestRouter(t)
	for _, u := range []testUser{user1, staffUser} {
		t.Run(u.role, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, "/api/reports/notifications/summary", makeToken(t, u), "")
			expectStatus(t, w, http.StatusForbidden)
		})
	}
}

func TestNotificationSummary_Unauthorized(t *testing.T) {
	r := newTestRouter(t)
	w := sendRequest(r, http.MethodGet, "/api/reports/notifications/summary", "", "")
	expectStatus(t, w, http.StatusUnauthorized)
}
