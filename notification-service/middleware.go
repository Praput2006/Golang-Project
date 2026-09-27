package main

import (
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ---------- Keycloak middleware (ก๊อปจาก auth-service/middleware.go ให้ทุก service ตรวจแบบเดียวกัน) ----------

type RealmAccess struct {
	Roles []string `json:"roles"`
}

// Claims คือข้อมูลที่อยู่ใน access token ของ Keycloak
// Subject (sub) คือ id ของ user ใน Keycloak ตรงกับคอลัมน์ keycloak_id ใน auth_db
type Claims struct {
	jwt.RegisteredClaims
	PreferredUsername string      `json:"preferred_username"`
	Email             string      `json:"email"`
	RealmAccess       RealmAccess `json:"realm_access"`
}

func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, r := range roles {
		if slices.Contains(c.RealmAccess.Roles, r) {
			return true
		}
	}
	return false
}

const claimsKey = "claims"

// AuthMiddleware ตรวจ Bearer token ว่าออกโดย Keycloak จริง (ลายเซ็นถูก, ยังไม่หมดอายุ, issuer ตรง)
func AuthMiddleware(keyfunc jwt.Keyfunc, issuer string) gin.HandlerFunc {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
	}
	if issuer != "" {
		opts = append(opts, jwt.WithIssuer(issuer))
	}
	parser := jwt.NewParser(opts...)

	return func(c *gin.Context) {
		tokenString, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		claims := &Claims{}
		if _, err := parser.ParseWithClaims(tokenString, claims, keyfunc); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set(claimsKey, claims)
		c.Next()
	}
}

// RequireRole ให้ผ่านเฉพาะคนที่มี role อย่างน้อยหนึ่งตัวในรายการ ต้องวางหลัง AuthMiddleware
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := currentClaims(c)
		if claims == nil || !claims.HasAnyRole(roles...) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}

func currentClaims(c *gin.Context) *Claims {
	v, _ := c.Get(claimsKey)
	claims, _ := v.(*Claims)
	return claims
}

// ---------- ส่วนเฉพาะของ notification-service ----------

// InternalKeyMiddleware ใช้กับ /internal/* ที่ service อื่นเรียกกันเองใน Docker network (Kong ไม่เปิดออกข้างนอก)
func InternalKeyMiddleware(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Internal-Key")
		if key == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid internal key"})
			return
		}
		c.Next()
	}
}
