package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// authRequired reads the Bearer token Kong forwards through unchanged,
// decodes its claims (see claims.go), and stores user_id/username/role on
// the Gin context for handlers and requireRole() to use.
func authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := ParseClaims(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token: " + err.Error()})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// requireRole aborts with 403 unless authRequired() already put one of the
// given roles on the context. Every service in the team should implement
// this the same way (same claim names, same middleware shape) so a booking
// developed here behaves identically once Auth Service issues real tokens.
func requireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)
		if !allowed[roleStr] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions for this action"})
			return
		}
		c.Next()
	}
}
