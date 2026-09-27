package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// 6. Special (Event-driven) — POST /internal/events
// Booking Service เรียกเมื่อการจองถูกสร้าง/อนุมัติ/ปฏิเสธ แล้วสร้าง notification ให้เจ้าของการจองอัตโนมัติ (ดู EVENTS.md)
func receiveBookingEvent(c *gin.Context) {
	var req BookingEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	notifType, title, message := bookingEventMessage(req)
	n := Notification{
		UserID:  req.UserID,
		Title:   title,
		Message: message,
		Type:    notifType,
	}
	if err := db.Create(&n).Error; err != nil {
		log.Printf("booking event %s #%d: %v", req.Event, req.BookingID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot create notification"})
		return
	}

	c.JSON(http.StatusCreated, n)
}

// bookingEventMessage แปลง event เป็น type, หัวข้อ และข้อความภาษาไทย
func bookingEventMessage(e BookingEventRequest) (notifType, title, message string) {
	booking := describeBooking(e)
	switch e.Event {
	case EventBookingApproved:
		return TypeBookingApproved, "การจองได้รับการอนุมัติ", booking + " ได้รับการอนุมัติแล้ว"
	case EventBookingRejected:
		message = booking + " ถูกปฏิเสธ"
		if reason := strings.TrimSpace(e.RejectReason); reason != "" {
			message += " เหตุผล: " + reason
		}
		return TypeBookingRejected, "การจองถูกปฏิเสธ", message
	default: // EventBookingCreated (binding ตรวจค่า event ให้แล้ว)
		return TypeBookingCreated, "สร้างการจองสำเร็จ", booking + " ถูกสร้างแล้ว กรุณารอการอนุมัติ"
	}
}

// describeBooking สร้างข้อความเช่น "การจอง #12 (ห้อง 301 วันที่ 2026-10-01 เวลา 09:00-11:00)"
func describeBooking(e BookingEventRequest) string {
	var details []string
	switch {
	case e.FacilityName != "":
		details = append(details, e.FacilityName)
	case e.FacilityID != 0:
		details = append(details, fmt.Sprintf("ห้อง/พื้นที่ #%d", e.FacilityID))
	}
	if e.Date != "" {
		details = append(details, "วันที่ "+e.Date)
	}
	if e.StartTime != "" && e.EndTime != "" {
		details = append(details, fmt.Sprintf("เวลา %s-%s", e.StartTime, e.EndTime))
	}

	s := fmt.Sprintf("การจอง #%d", e.BookingID)
	if len(details) > 0 {
		s += " (" + strings.Join(details, " ") + ")"
	}
	return s
}
