package main

import (
	"crypto/rand"
	"crypto/rsa"
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
	r.POST("/internal", InternalKeyMiddleware(testInternalKey), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func signClaims(t *testing.T, claims Claims, key *rsa.PrivateKey) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
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
	w := sendRequest(r, http.MethodGet, "/me", signClaims(t, validClaims(), testKey), "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "kc-123" {
		t.Errorf("subject = %q, want kc-123", w.Body.String())
	}
}

func TestAuthMiddleware_RejectsBadTokens(t *testing.T) {
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	expired := validClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))

	wrongIssuer := validClaims()
	wrongIssuer.Issuer = "http://evil.test/realms/unibook"

	noExpiry := validClaims()
	noExpiry.ExpiresAt = nil

	tests := []struct {
		name  string
		token string
	}{
		{"no token", ""},
		{"not a jwt", "abc.def.ghi"},
		{"signed by another key", signClaims(t, validClaims(), otherKey)},
		{"expired", signClaims(t, expired, testKey)},
		{"wrong issuer", signClaims(t, wrongIssuer, testKey)},
		{"no expiry", signClaims(t, noExpiry, testKey)},
	}

	r := newMiddlewareRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sendRequest(r, http.MethodGet, "/me", tt.token, "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestAuthMiddleware_RequiresBearerPrefix(t *testing.T) {
	r := newMiddlewareRouter()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", signClaims(t, validClaims(), testKey)) // ไม่มีคำว่า Bearer
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		role string
		want int
	}{
		{RoleUser, http.StatusForbidden},
		{RoleStaff, http.StatusOK},
		{RoleAdmin, http.StatusOK},
	}

	r := newMiddlewareRouter()
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			claims := validClaims()
			claims.RealmAccess.Roles = []string{tt.role}
			w := sendRequest(r, http.MethodGet, "/staff-only", signClaims(t, claims, testKey), "")
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestInternalKeyMiddleware(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want int
	}{
		{"correct key", testInternalKey, http.StatusOK},
		{"wrong key", "wrong-key", http.StatusUnauthorized},
		{"no key", "", http.StatusUnauthorized},
	}

	r := newMiddlewareRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sendInternal(r, "/internal", tt.key, "")
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestInternalKeyMiddleware_EmptyConfigRejectsAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal", InternalKeyMiddleware(""), func(c *gin.Context) { c.Status(http.StatusOK) })

	if w := sendInternal(r, "/internal", "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when INTERNAL_API_KEY is not set", w.Code)
	}
}
