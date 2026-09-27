package main

import (
	"github.com/gin-gonic/gin"
)

func setupRouter(auth gin.HandlerFunc) *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	registerSwagger(r)

	api := r.Group("/api")

	// ไม่ต้อง login
	api.POST("/auth/register", register)

	// ต้อง login
	authed := api.Group("", auth)
	authed.GET("/users/me", getMe)
	authed.GET("/users/:id", RequireRole(RoleAdmin, RoleStaff), getUser)

	// เฉพาะ ADMIN
	admin := authed.Group("", RequireRole(RoleAdmin))
	admin.GET("/users", listUsers)
	admin.PUT("/users/:id", updateUser)
	admin.PUT("/users/:id/deactivate", deactivateUser)
	admin.PUT("/users/:id/role", changeRole)
	admin.GET("/reports/users/summary", userSummary)

	return r
}
