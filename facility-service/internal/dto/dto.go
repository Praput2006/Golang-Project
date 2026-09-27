package dto

import (
	"time"

	"facility-service/internal/model"
)

const (
	DateLayout  = "2006-01-02"
	ClockLayout = "15:04"
)

// ---------- Requests ----------

type CreateFacilityRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Type        string `json:"type" binding:"required,oneof=MEETING_ROOM HALL LAB CLASSROOM SPORTS OTHER"`
	Capacity    int    `json:"capacity" binding:"required,gt=0"`
	Description string `json:"description"`
	Status      string `json:"status" binding:"omitempty,oneof=AVAILABLE UNAVAILABLE"`
}

// ใช้ pointer เพื่อให้แก้เฉพาะ field ที่ส่งมา (partial update)
type UpdateFacilityRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=100"`
	Type        *string `json:"type" binding:"omitempty,oneof=MEETING_ROOM HALL LAB CLASSROOM SPORTS OTHER"`
	Capacity    *int    `json:"capacity" binding:"omitempty,gt=0"`
	Description *string `json:"description"`
	Status      *string `json:"status" binding:"omitempty,oneof=AVAILABLE UNAVAILABLE"`
}

type ListFacilityQuery struct {
	Q           string `form:"q"`
	Type        string `form:"type" binding:"omitempty,oneof=MEETING_ROOM HALL LAB CLASSROOM SPORTS OTHER"`
	Status      string `form:"status" binding:"omitempty,oneof=AVAILABLE UNAVAILABLE"`
	MinCapacity int    `form:"min_capacity" binding:"omitempty,gte=0"`
	Page        int    `form:"page" binding:"omitempty,gte=1"`
	Limit       int    `form:"limit" binding:"omitempty,gte=1,lte=100"`
}

type CreateAvailabilityRequest struct {
	Date      string `json:"date" binding:"required,datetime=2006-01-02"`
	StartTime string `json:"start_time" binding:"required,datetime=15:04"`
	EndTime   string `json:"end_time" binding:"required,datetime=15:04"`
}

type UsageReportQuery struct {
	From string `form:"from" binding:"omitempty,datetime=2006-01-02"`
	To   string `form:"to" binding:"omitempty,datetime=2006-01-02"`
}

// ---------- Responses ----------

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type FacilityResponse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Capacity    int       `json:"capacity"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FacilityDetailResponse struct {
	FacilityResponse
	UpcomingAvailability []AvailabilityResponse `json:"upcoming_availability"`
}

type AvailabilityResponse struct {
	ID         int64     `json:"id"`
	FacilityID int64     `json:"facility_id"`
	Date       string    `json:"date"`
	StartTime  string    `json:"start_time"`
	EndTime    string    `json:"end_time"`
	CreatedAt  time.Time `json:"created_at"`
}

type PagedResponse[T any] struct {
	Data       []T   `json:"data"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type FacilityUsage struct {
	FacilityID     int64   `json:"facility_id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Capacity       int     `json:"capacity"`
	Status         string  `json:"status"`
	SlotCount      int64   `json:"slot_count"`
	AvailableHours float64 `json:"available_hours"`
}

type UsageReportResponse struct {
	From                string          `json:"from"`
	To                  string          `json:"to"`
	TotalFacilities     int             `json:"total_facilities"`
	TotalCapacity       int             `json:"total_capacity"`
	TotalSlots          int64           `json:"total_slots"`
	TotalAvailableHours float64         `json:"total_available_hours"`
	ByType              map[string]int  `json:"by_type"`
	ByStatus            map[string]int  `json:"by_status"`
	Facilities          []FacilityUsage `json:"facilities"`
}

// ---------- Mappers ----------

func ToFacilityResponse(f *model.Facility) FacilityResponse {
	return FacilityResponse{
		ID: f.ID, Name: f.Name, Type: f.Type, Capacity: f.Capacity,
		Description: f.Description, Status: f.Status,
		CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

func ToAvailabilityResponse(a *model.FacilityAvailability) AvailabilityResponse {
	return AvailabilityResponse{
		ID: a.ID, FacilityID: a.FacilityID,
		Date:      a.Date.Format(DateLayout),
		StartTime: clock(a.StartTime),
		EndTime:   clock(a.EndTime),
		CreatedAt: a.CreatedAt,
	}
}

// Postgres คืน TIME มาเป็น "09:00:00" หรือ "09:00:00.000000" → ตัดให้เหลือ "09:00"
func clock(s string) string {
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}
