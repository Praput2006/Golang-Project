package router

import (
	"net/http"

	"facility-service/docs"
	"facility-service/internal/handler"
	mw "facility-service/internal/middleware"

	"github.com/gin-gonic/gin"
)

// auth ถูก inject เข้ามา เพื่อให้ test ใช้ fake middleware แทน Keycloak ได้
func New(h *handler.FacilityHandler, auth gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	docs.Register(r) // GET /docs (Swagger UI), GET /docs/openapi.yaml

	staffOrAdmin := mw.RequireRoles(mw.RoleStaff, mw.RoleAdmin)
	adminOnly := mw.RequireRoles(mw.RoleAdmin)

	api := r.Group("/api", auth)

	f := api.Group("/facilities")
	f.POST("", staffOrAdmin, h.Create)                           // 1. Create
	f.GET("", h.List)                                            // 2. List/Search
	f.GET("/:id", h.GetByID)                                     // 3. Read one
	f.PUT("/:id", staffOrAdmin, h.Update)                        // 4. Update
	f.DELETE("/:id", adminOnly, h.Delete)                        // 5. Delete
	f.POST("/:id/availability", staffOrAdmin, h.AddAvailability) // 6. Special

	api.GET("/reports/facilities/usage", adminOnly, h.UsageReport) // 7. Report

	return r
}
