package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"facility-service/internal/dto"
	"facility-service/internal/service"

	"github.com/gin-gonic/gin"
)

type FacilityHandler struct {
	svc service.FacilityService
}

func NewFacilityHandler(svc service.FacilityService) *FacilityHandler {
	return &FacilityHandler{svc: svc}
}

// POST /api/facilities  (STAFF, ADMIN)
func (h *FacilityHandler) Create(c *gin.Context) {
	var req dto.CreateFacilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	res, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// GET /api/facilities?q=&type=&status=&min_capacity=&page=&limit=
func (h *FacilityHandler) List(c *gin.Context) {
	var q dto.ListFacilityQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		badRequest(c, err.Error())
		return
	}
	res, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GET /api/facilities/:id
func (h *FacilityHandler) GetByID(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	res, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// PUT /api/facilities/:id  (STAFF, ADMIN)
func (h *FacilityHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateFacilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	res, err := h.svc.Update(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// DELETE /api/facilities/:id?force=true  (ADMIN)
func (h *FacilityHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	force := c.Query("force") == "true"
	if err := h.svc.Delete(c.Request.Context(), id, force); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /api/facilities/:id/availability  (STAFF, ADMIN)
func (h *FacilityHandler) AddAvailability(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.CreateAvailabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}
	res, err := h.svc.AddAvailability(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// GET /api/reports/facilities/usage?from=&to=  (ADMIN)
func (h *FacilityHandler) UsageReport(c *gin.Context) {
	var q dto.UsageReportQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		badRequest(c, err.Error())
		return
	}
	res, err := h.svc.UsageReport(c.Request.Context(), q)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ---------- helpers ----------

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "BAD_REQUEST", Message: msg})
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBadRequest):
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "BAD_REQUEST", Message: err.Error()})
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "NOT_FOUND", Message: err.Error()})
	case errors.Is(err, service.ErrConflict):
		c.JSON(http.StatusConflict, dto.ErrorResponse{Error: "CONFLICT", Message: err.Error()})
	default:
		log.Printf("internal error: %v", err)
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "INTERNAL_ERROR", Message: "something went wrong"})
	}
}
