package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"facility-service/internal/dto"
	"facility-service/internal/handler"
	mw "facility-service/internal/middleware"
	"facility-service/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ทดสอบ routing + role check + การแปลง error เป็น HTTP status
// โดยใช้ fake service และ fake auth (ไม่ต้องมี DB / Keycloak)

type fakeService struct{}

func (fakeService) Create(_ context.Context, req dto.CreateFacilityRequest) (*dto.FacilityResponse, error) {
	return &dto.FacilityResponse{ID: 1, Name: req.Name, Type: req.Type, Capacity: req.Capacity, Status: "AVAILABLE"}, nil
}
func (fakeService) List(_ context.Context, q dto.ListFacilityQuery) (*dto.PagedResponse[dto.FacilityResponse], error) {
	return &dto.PagedResponse[dto.FacilityResponse]{Data: []dto.FacilityResponse{}, Page: 1, Limit: 10}, nil
}
func (fakeService) GetByID(_ context.Context, id int64) (*dto.FacilityDetailResponse, error) {
	if id == 999 {
		return nil, fmt.Errorf("%w: facility 999", service.ErrNotFound)
	}
	return &dto.FacilityDetailResponse{FacilityResponse: dto.FacilityResponse{ID: id}}, nil
}
func (fakeService) Update(_ context.Context, id int64, _ dto.UpdateFacilityRequest) (*dto.FacilityResponse, error) {
	return &dto.FacilityResponse{ID: id}, nil
}
func (fakeService) Delete(_ context.Context, _ int64, force bool) error {
	if !force {
		return fmt.Errorf("%w: has upcoming slots", service.ErrConflict)
	}
	return nil
}
func (fakeService) AddAvailability(_ context.Context, id int64, req dto.CreateAvailabilityRequest) (*dto.AvailabilityResponse, error) {
	return &dto.AvailabilityResponse{ID: 1, FacilityID: id, Date: req.Date, StartTime: req.StartTime, EndTime: req.EndTime}, nil
}
func (fakeService) UsageReport(_ context.Context, _ dto.UsageReportQuery) (*dto.UsageReportResponse, error) {
	return &dto.UsageReportResponse{ByType: map[string]int{}, ByStatus: map[string]int{}}, nil
}

// fake auth: อ่าน role จาก header X-Test-Roles แทน JWT
func fakeAuth(c *gin.Context) {
	roles := c.GetHeader("X-Test-Roles")
	if roles == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "UNAUTHORIZED"})
		return
	}
	mw.SetIdentity(c, "test-sub", "tester", strings.Split(roles, ","))
	c.Next()
}

func setup() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return New(handler.NewFacilityHandler(fakeService{}), fakeAuth)
}

func do(r *gin.Engine, method, path string, body any, roles string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if roles != "" {
		req.Header.Set("X-Test-Roles", roles)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var validFacility = map[string]any{"name": "Room 301", "type": "MEETING_ROOM", "capacity": 30}

func TestUnauthenticated(t *testing.T) {
	assert.Equal(t, http.StatusUnauthorized, do(setup(), "GET", "/api/facilities", nil, "").Code)
}

func TestCreate(t *testing.T) {
	r := setup()
	assert.Equal(t, http.StatusForbidden, do(r, "POST", "/api/facilities", validFacility, "USER").Code)
	assert.Equal(t, http.StatusCreated, do(r, "POST", "/api/facilities", validFacility, "STAFF").Code)
	assert.Equal(t, http.StatusCreated, do(r, "POST", "/api/facilities", validFacility, "ADMIN").Code)

	bad := map[string]any{"name": "X", "type": "KITCHEN", "capacity": 0}
	assert.Equal(t, http.StatusBadRequest, do(r, "POST", "/api/facilities", bad, "STAFF").Code)
}

func TestList(t *testing.T) {
	r := setup()
	assert.Equal(t, http.StatusOK, do(r, "GET", "/api/facilities?type=LAB&page=1&limit=5", nil, "USER").Code)
	assert.Equal(t, http.StatusBadRequest, do(r, "GET", "/api/facilities?limit=500", nil, "USER").Code)
}

func TestGetByID(t *testing.T) {
	r := setup()
	assert.Equal(t, http.StatusOK, do(r, "GET", "/api/facilities/1", nil, "USER").Code)
	assert.Equal(t, http.StatusNotFound, do(r, "GET", "/api/facilities/999", nil, "USER").Code)
	assert.Equal(t, http.StatusBadRequest, do(r, "GET", "/api/facilities/abc", nil, "USER").Code)
}

func TestUpdate(t *testing.T) {
	r := setup()
	body := map[string]any{"capacity": 50}
	assert.Equal(t, http.StatusForbidden, do(r, "PUT", "/api/facilities/1", body, "USER").Code)
	assert.Equal(t, http.StatusOK, do(r, "PUT", "/api/facilities/1", body, "STAFF").Code)
}

func TestDelete(t *testing.T) {
	r := setup()
	assert.Equal(t, http.StatusForbidden, do(r, "DELETE", "/api/facilities/1", nil, "STAFF").Code)
	assert.Equal(t, http.StatusConflict, do(r, "DELETE", "/api/facilities/1", nil, "ADMIN").Code)
	assert.Equal(t, http.StatusNoContent, do(r, "DELETE", "/api/facilities/1?force=true", nil, "ADMIN").Code)
}

func TestAddAvailability(t *testing.T) {
	r := setup()
	ok := map[string]any{"date": "2026-10-05", "start_time": "09:00", "end_time": "12:00"}
	bad := map[string]any{"date": "05/10/2026", "start_time": "9am", "end_time": "12:00"}
	assert.Equal(t, http.StatusForbidden, do(r, "POST", "/api/facilities/1/availability", ok, "USER").Code)
	assert.Equal(t, http.StatusCreated, do(r, "POST", "/api/facilities/1/availability", ok, "STAFF").Code)
	assert.Equal(t, http.StatusBadRequest, do(r, "POST", "/api/facilities/1/availability", bad, "STAFF").Code)
}

func TestUsageReport(t *testing.T) {
	r := setup()
	assert.Equal(t, http.StatusForbidden, do(r, "GET", "/api/reports/facilities/usage", nil, "STAFF").Code)
	assert.Equal(t, http.StatusOK, do(r, "GET", "/api/reports/facilities/usage?from=2026-09-01&to=2026-09-30", nil, "ADMIN").Code)
	assert.Equal(t, http.StatusBadRequest, do(r, "GET", "/api/reports/facilities/usage?from=bad", nil, "ADMIN").Code)
}
