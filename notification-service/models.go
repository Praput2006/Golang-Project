package main

import (
	"time"
)

const (
	RoleUser  = "USER"
	RoleStaff = "STAFF"
	RoleAdmin = "ADMIN"

	// ประเภทการแจ้งเตือน (ตาม Database Design + GENERAL สำหรับ STAFF/ADMIN ส่งเอง)
	TypeGeneral          = "GENERAL"
	TypeBookingCreated   = "BOOKING_CREATED"
	TypeBookingApproved  = "BOOKING_APPROVED"
	TypeBookingRejected  = "BOOKING_REJECTED"
	TypeBookingCancelled = "BOOKING_CANCELLED"

	// event ที่ Booking Service ส่งมา (ดู EVENTS.md)
	EventBookingCreated  = "booking.created"
	EventBookingApproved = "booking.approved"
	EventBookingRejected = "booking.rejected"
)

var allTypes = []string{TypeGeneral, TypeBookingCreated, TypeBookingApproved, TypeBookingRejected, TypeBookingCancelled}

var validTypes = map[string]bool{
	TypeGeneral: true, TypeBookingCreated: true, TypeBookingApproved: true,
	TypeBookingRejected: true, TypeBookingCancelled: true,
}

// Notification ตรงกับตาราง notifications ที่สร้างใน migrations/001_create_notifications_table.sql
type Notification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"` // users.id ใน auth_db (ผู้รับการแจ้งเตือน)
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Type      string    `json:"type"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateNotificationRequest struct {
	UserID  int64  `json:"user_id" binding:"required,min=1"`
	Title   string `json:"title" binding:"required,max=100"`
	Message string `json:"message" binding:"required"`
	Type    string `json:"type" binding:"omitempty,oneof=GENERAL BOOKING_CREATED BOOKING_APPROVED BOOKING_REJECTED BOOKING_CANCELLED"`
}

// BookingEventRequest ใช้ชื่อ field เดียวกับตาราง bookings ใน booking_db
type BookingEventRequest struct {
	Event        string `json:"event" binding:"required,oneof=booking.created booking.approved booking.rejected"`
	BookingID    int64  `json:"booking_id" binding:"required,min=1"`
	UserID       int64  `json:"user_id" binding:"required,min=1"` // เจ้าของการจอง
	FacilityID   int64  `json:"facility_id"`
	FacilityName string `json:"facility_name"`
	Date         string `json:"date"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	RejectReason string `json:"reject_reason"`
}

type NotificationSummary struct {
	Total    int64            `json:"total"`
	Read     int64            `json:"read"`
	Unread   int64            `json:"unread"`
	ReadRate float64          `json:"read_rate"` // เปอร์เซ็นต์ที่อ่านแล้ว ทศนิยม 2 ตำแหน่ง
	ByType   map[string]int64 `json:"by_type"`
}
