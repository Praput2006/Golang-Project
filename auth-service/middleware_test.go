package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// newMiddlewareRouter สร้าง router เล็ก ๆ ไว้ทดสอบ middleware อย่างเดียว
func newMiddlewareRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := AuthMiddleware(testKeyfunc, testIssuer)

	r.GET("/me", auth, func(c *gin.Context) {
		c.String(http.StatusOK, currentClaims(c).Subject)
	})
	r.GET("/staff-only", auth, RequireRole(RoleStaff, RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func signClaims(t *testing.T, claims Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func validClaims() Claims {
	return Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "kc-123",
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		RealmAccess: RealmAccess{Roles: []string{RoleUser}},
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	r := newMiddlewareRouter()
	w := sendRequest(r, http.MethodGet, "/me", signClaims(t, validClaims()), "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "kc-123" {
		t.Errorf("subject = %q, want kc-123", w.Body.String())
	}
}

func TestAuthMiddleware_RejectsBadTokens(t *testing.T) {
	r := newMiddlewareRouter()

	expired := validClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))

	wrongIssuer := validClaims()
	wrongIssuer.Issuer = "http://evil.example/realms/unibook"

	noExpiry := validClaims()
	noExpiry.ExpiresAt = nil

	// token ที่เซ็นด้วย HS256 (ใช้ secret) ต้องไม่ผ่าน เพราะเรารับเฉพาะ RS256 จาก Keycloak
	hsToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims()).SignedString([]byte("guess"))

	tests := []struct{ name, token string }{
		{"malformed", "not-a-jwt"},
		{"expired", signClaims(t, expired)},
		{"wrong issuer", signClaims(t, wrongIssuer)},
		{"no expiry", signClaims(t, noExpiry)},
		{"wrong algorithm", hsToken},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, "/me", tc.token, "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestAuthMiddleware_NonBearerScheme(t *testing.T) {
	r := newMiddlewareRouter()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireRole(t *testing.T) {
	r := newMiddlewareRouter()

	tests := []struct {
		role string
		want int
	}{
		{RoleUser, http.StatusForbidden},
		{RoleStaff, http.StatusOK},
		{RoleAdmin, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			claims := validClaims()
			claims.RealmAccess.Roles = []string{"default-roles-unibook", tc.role}

			w := sendRequest(r, http.MethodGet, "/staff-only", signClaims(t, claims), "")
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
