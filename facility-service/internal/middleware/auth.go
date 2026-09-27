package middleware

import (
	"context"
	"net/http"
	"strings"

	"facility-service/internal/dto"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Role ที่ตั้งไว้ใน Keycloak realm (ต้องตรงกันทุก service)
const (
	RoleUser  = "USER"
	RoleStaff = "STAFF"
	RoleAdmin = "ADMIN"
)

const (
	ctxSubject  = "auth.sub"
	ctxUsername = "auth.username"
	ctxRoles    = "auth.roles"
)

type KeycloakClaims struct {
	jwt.RegisteredClaims
	PreferredUsername string `json:"preferred_username"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// NewKeycloakAuth ตรวจ Bearer token ด้วย public key (JWKS) ของ Keycloak
// issuer เว้นว่างได้ถ้ายังไม่อยากตรวจ (เช่นตอน dev ที่ token ออกจาก localhost แต่ service เรียก keycloak:8080)
func NewKeycloakAuth(ctx context.Context, jwksURL, issuer string) (gin.HandlerFunc, error) {
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, err
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
	}
	if issuer != "" {
		opts = append(opts, jwt.WithIssuer(issuer))
	}

	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			unauthorized(c, "missing bearer token")
			return
		}
		claims := &KeycloakClaims{}
		token, err := jwt.ParseWithClaims(strings.TrimPrefix(header, "Bearer "), claims, k.Keyfunc, opts...)
		if err != nil || !token.Valid {
			unauthorized(c, "invalid or expired token")
			return
		}
		SetIdentity(c, claims.Subject, claims.PreferredUsername, claims.RealmAccess.Roles)
		c.Next()
	}, nil
}

// RequireRoles ผ่านเมื่อผู้ใช้มีอย่างน้อยหนึ่ง role ในรายการ
func RequireRoles(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, have := range Roles(c) {
			for _, want := range allowed {
				if have == want {
					c.Next()
					return
				}
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, dto.ErrorResponse{
			Error:   "FORBIDDEN",
			Message: "requires one of roles: " + strings.Join(allowed, ", "),
		})
	}
}

func SetIdentity(c *gin.Context, sub, username string, roles []string) {
	c.Set(ctxSubject, sub)
	c.Set(ctxUsername, username)
	c.Set(ctxRoles, roles)
}

func Subject(c *gin.Context) string  { return c.GetString(ctxSubject) }
func Username(c *gin.Context) string { return c.GetString(ctxUsername) }
func Roles(c *gin.Context) []string  { return c.GetStringSlice(ctxRoles) }

func unauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "UNAUTHORIZED", Message: msg})
}
