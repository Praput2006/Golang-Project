package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// test ในไฟล์นี้ทดสอบเฉพาะกรณีที่ตอบกลับก่อนถึงฐานข้อมูลและ Keycloak
// (ไม่มี token, role ไม่พอ, ข้อมูลที่ส่งมาผิด) จึงรันได้ทันทีโดยไม่ต้องเปิด Docker

const testIssuer = "http://keycloak.test/realms/unibook"

// testKey ใช้แทน private key ของ Keycloak สำหรับเซ็น token ปลอมใน test
var testKey, _ = rsa.GenerateKey(rand.Reader, 2048)

func testKeyfunc(*jwt.Token) (any, error) {
	return &testKey.PublicKey, nil
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return setupRouter(AuthMiddleware(testKeyfunc, testIssuer))
}

// makeToken สร้าง access token ปลอมแบบเดียวกับที่ Keycloak ออกให้ user ที่มี role นี้
func makeToken(t *testing.T, role string) string {
	t.Helper()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "kc-test-user",
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		RealmAccess: RealmAccess{Roles: []string{role}},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func sendRequest(r *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHealth(t *testing.T) {
	r := newTestRouter()
	w := sendRequest(r, http.MethodGet, "/health", "", "")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ---------- 1. POST /api/auth/register ----------

func TestRegister_InvalidInput(t *testing.T) {
	r := newTestRouter()

	tests := []struct {
		name string
		body string
	}{
		{"missing email", `{"username":"nathida","password":"Secret123!","first_name":"N","last_name":"K"}`},
		{"invalid email", `{"username":"nathida","email":"not-an-email","password":"Secret123!","first_name":"N","last_name":"K"}`},
		{"short password", `{"username":"nathida","email":"n@ubu.ac.th","password":"short","first_name":"N","last_name":"K"}`},
		{"short username", `{"username":"ab","email":"n@ubu.ac.th","password":"Secret123!","first_name":"N","last_name":"K"}`},
		{"username with space", `{"username":"nat hida","email":"n@ubu.ac.th","password":"Secret123!","first_name":"N","last_name":"K"}`},
		{"missing first name", `{"username":"nathida","email":"n@ubu.ac.th","password":"Secret123!","last_name":"K"}`},
		{"not json", `hello`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := sendRequest(r, http.MethodPost, "/api/auth/register", "", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

// ---------- ต้อง login ก่อน (401) ----------

func TestProtectedRoutes_RequireToken(t *testing.T) {
	r := newTestRouter()

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/users/me"},
		{http.MethodGet, "/api/users"},
		{http.MethodGet, "/api/users/1"},
		{http.MethodPut, "/api/users/1"},
		{http.MethodPut, "/api/users/1/deactivate"},
		{http.MethodPut, "/api/users/1/role"},
		{http.MethodGet, "/api/reports/users/summary"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := sendRequest(r, rt.method, rt.path, "", "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

// ---------- role ไม่พอ (403) ----------

func TestAdminRoutes_ForbiddenForUserAndStaff(t *testing.T) {
	r := newTestRouter()

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/users"},
		{http.MethodPut, "/api/users/1"},
		{http.MethodPut, "/api/users/1/deactivate"},
		{http.MethodPut, "/api/users/1/role"},
		{http.MethodGet, "/api/reports/users/summary"},
	}
	for _, role := range []string{RoleUser, RoleStaff} {
		token := makeToken(t, role)
		for _, rt := range routes {
			t.Run(role+" "+rt.method+" "+rt.path, func(t *testing.T) {
				w := sendRequest(r, rt.method, rt.path, token, "")
				if w.Code != http.StatusForbidden {
					t.Errorf("status = %d, want 403", w.Code)
				}
			})
		}
	}
}

func TestGetUser_ForbiddenForUser(t *testing.T) {
	r := newTestRouter()
	w := sendRequest(r, http.MethodGet, "/api/users/1", makeToken(t, RoleUser), "")
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// ---------- ข้อมูลที่ส่งมาผิด (400) ----------

func TestListUsers_InvalidFilter(t *testing.T) {
	r := newTestRouter()
	token := makeToken(t, RoleAdmin)

	for _, path := range []string{"/api/users?role=SUPERUSER", "/api/users?status=BANNED"} {
		t.Run(path, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, path, token, "")
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestInvalidUserID(t *testing.T) {
	r := newTestRouter()
	admin := makeToken(t, RoleAdmin)
	validUpdate := `{"email":"x@ubu.ac.th","first_name":"X","last_name":"Y"}`

	tests := []struct {
		name, method, path, body string
	}{
		{"get", http.MethodGet, "/api/users/abc", ""},
		{"get zero", http.MethodGet, "/api/users/0", ""},
		{"update", http.MethodPut, "/api/users/abc", validUpdate},
		{"deactivate", http.MethodPut, "/api/users/abc/deactivate", ""},
		{"change role", http.MethodPut, "/api/users/abc/role", `{"role":"STAFF"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := sendRequest(r, tc.method, tc.path, admin, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestUpdateUser_InvalidBody(t *testing.T) {
	r := newTestRouter()
	admin := makeToken(t, RoleAdmin)

	tests := []struct{ name, body string }{
		{"invalid email", `{"email":"bad","first_name":"X","last_name":"Y"}`},
		{"missing last name", `{"email":"x@ubu.ac.th","first_name":"X"}`},
		{"empty body", `{}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := sendRequest(r, http.MethodPut, "/api/users/1", admin, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestChangeRole_InvalidRole(t *testing.T) {
	r := newTestRouter()
	admin := makeToken(t, RoleAdmin)

	tests := []struct{ name, body string }{
		{"unknown role", `{"role":"SUPERUSER"}`},
		{"lower case", `{"role":"staff"}`},
		{"missing role", `{}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := sendRequest(r, http.MethodPut, "/api/users/1/role", admin, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}
