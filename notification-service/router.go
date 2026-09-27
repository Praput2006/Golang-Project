package main

import (
	"github.com/gin-gonic/gin"
)

func setupRouter(auth, internal gin.HandlerFunc) *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	registerSwagger(r)

	api := r.Group("/api")

	// ต้อง login (ทุก role เห็น/อ่าน/ลบได้เฉพาะของตัวเอง)
	authed := api.Group("", auth)
	authed.GET("/notifications", listNotifications)
	authed.GET("/notifications/:id", getNotification)
	authed.PUT("/notifications/:id/read", markAsRead)
	authed.DELETE("/notifications/:id", deleteNotification)

	// เฉพาะ STAFF, ADMIN
	authed.POST("/notifications", RequireRole(RoleStaff, RoleAdmin), createNotification)

	// เฉพาะ ADMIN
	authed.GET("/reports/notifications/summary", RequireRole(RoleAdmin), notificationSummary)

	// service-to-service เท่านั้น — ห้ามเปิด /internal ผ่าน Kong
	r.POST("/internal/events", internal, receiveBookingEvent)

	return r
}
