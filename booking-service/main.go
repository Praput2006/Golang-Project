package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ---- Booking status values (matches Database__2_.pdf: bookings.status) ----
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
)

// Booking matches the `bookings` table in booking_db exactly as specified
// in the team's Database Design doc (booking_db, table: bookings).
type Booking struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	UserID     uint   `gorm:"not null;index" json:"user_id"`
	FacilityID uint   `gorm:"not null;index" json:"facility_id"`
	Date       string `gorm:"type:date;not null;index" json:"date"`
	// NOTE: the raw type must be "time without time zone", not just "time" —
	// GORM treats the bare string "time" as its own abstract DataType (the
	// one used for time.Time fields) and silently maps it to timestamptz
	// instead of passing it through as a literal Postgres type. Verified
	// against a real Postgres instance while building this service.
	StartTime    string    `gorm:"type:time without time zone;not null" json:"start_time"`
	EndTime      string    `gorm:"type:time without time zone;not null" json:"end_time"`
	Status       string    `gorm:"type:varchar(20);not null;default:PENDING;index" json:"status"`
	RejectReason *string   `gorm:"type:varchar(255)" json:"reject_reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

var db *gorm.DB

func initDB() {
	var err error
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbPort := os.Getenv("DB_PORT")

	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5432"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		dbHost, dbUser, dbPassword, dbName, dbPort)
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&Booking{}); err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}
}

// ---------------------------------------------------------------------
// 1. Create — POST /api/bookings (USER)
// ---------------------------------------------------------------------

type CreateBookingRequest struct {
	FacilityID uint   `json:"facility_id" binding:"required"`
	Date       string `json:"date" binding:"required"`
	StartTime  string `json:"start_time" binding:"required"`
	EndTime    string `json:"end_time" binding:"required"`
}

func createBooking(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req CreateBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be in YYYY-MM-DD format"})
		return
	}
	if _, _, err := parseRange(req.StartTime, req.EndTime); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the facility exists via Facility Service, when configured.
	// See facility.go: if FACILITY_SERVICE_URL isn't set yet, this check is
	// skipped so booking-service can be developed/tested standalone.
	if _, err := fetchFacility(req.FacilityID); err != nil {
		switch err {
		case errFacilityServiceUnset:
			log.Printf("createBooking: FACILITY_SERVICE_URL not set, skipping facility validation")
		case errFacilityNotFound:
			c.JSON(http.StatusBadRequest, gin.H{"error": "facility not found"})
			return
		default:
			c.JSON(http.StatusFailedDependency, gin.H{"error": "Failed to verify facility: " + err.Error()})
			return
		}
	}

	// Reject requests that obviously conflict with an existing PENDING or
	// APPROVED booking on the same facility/date (the "special business
	// rule" / conflict case called out in the team's work breakdown).
	var existing []Booking
	if err := db.Where("facility_id = ? AND date = ? AND status IN ?",
		req.FacilityID, req.Date, []string{StatusPending, StatusApproved}).Find(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	for _, b := range existing {
		overlaps, err := timeRangesOverlap(req.StartTime, req.EndTime, b.StartTime, b.EndTime)
		if err != nil {
			// Don't let a malformed existing row silently defeat the
			// conflict check — log it and fail closed isn't appropriate
			// here (it's not this request's fault), so just skip that row.
			log.Printf("createBooking: could not compare against booking #%d: %v", b.ID, err)
			continue
		}
		if overlaps {
			c.JSON(http.StatusConflict, gin.H{"error": "This time slot conflicts with an existing booking"})
			return
		}
	}

	booking := Booking{
		UserID:     userID,
		FacilityID: req.FacilityID,
		Date:       req.Date,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Status:     StatusPending,
	}
	if err := db.Create(&booking).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create booking"})
		return
	}

	go notify(userID, "BOOKING_CREATED", "Booking submitted",
		fmt.Sprintf("Your booking request for facility #%d on %s has been submitted and is pending approval.",
			req.FacilityID, req.Date))

	c.JSON(http.StatusCreated, gin.H{
		"message": "Booking created successfully",
		"booking": gin.H{
			"id":          booking.ID,
			"facility_id": booking.FacilityID,
			"date":        normalizeDate(booking.Date),
			"start_time":  booking.StartTime,
			"end_time":    booking.EndTime,
			"status":      booking.Status,
		},
	})
}

// ---------------------------------------------------------------------
// 2. List — GET /api/bookings (USER sees own only, STAFF/ADMIN see all)
// ---------------------------------------------------------------------

// toBookingSummaries enriches bookings with the facility's display name
// (calling Facility Service, cached per-request) to match the response
// shape documented in project-2nd-step.pdf (facility_name, not facility_id).
func toBookingSummaries(bookings []Booking) []gin.H {
	cache := map[uint]string{}
	result := make([]gin.H, 0, len(bookings))
	for _, b := range bookings {
		name, ok := cache[b.FacilityID]
		if !ok {
			if fi, err := fetchFacility(b.FacilityID); err == nil {
				name = fi.Name
			} else {
				name = fmt.Sprintf("Facility #%d", b.FacilityID)
			}
			cache[b.FacilityID] = name
		}
		result = append(result, gin.H{
			"id":            b.ID,
			"facility_name": name,
			"date":          normalizeDate(b.Date),
			"status":        b.Status,
		})
	}
	return result
}

func listBookings(c *gin.Context) {
	roleStr, _ := c.MustGet("role").(string)
	userIDUint, _ := c.MustGet("user_id").(uint)

	query := db.Model(&Booking{})

	// USER can only ever see their own bookings through this endpoint;
	// STAFF/ADMIN see everything, further narrowed by the filters below.
	if roleStr == "USER" {
		query = query.Where("user_id = ?", userIDUint)
	}

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", strings.ToUpper(status))
	}
	if date := c.Query("date"); date != "" {
		query = query.Where("date = ?", date)
	}
	if facilityID := c.Query("facility_id"); facilityID != "" {
		query = query.Where("facility_id = ?", facilityID)
	}

	var total int64
	query.Count(&total)

	var bookings []Booking
	if err := query.Order("created_at DESC").Find(&bookings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  toBookingSummaries(bookings),
		"total": total,
	})
}

// ---------------------------------------------------------------------
// 3. Read own — GET /api/bookings/my (USER)
// ---------------------------------------------------------------------

func listMyBookings(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	query := db.Model(&Booking{}).Where("user_id = ?", userID)
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", strings.ToUpper(status))
	}

	var bookings []Booking
	if err := query.Order("created_at DESC").Find(&bookings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toBookingSummaries(bookings)})
}

// ---------------------------------------------------------------------
// 4. Update — PUT /api/bookings/:id/approve (STAFF/ADMIN)
// ---------------------------------------------------------------------

func approveBooking(c *gin.Context) {
	id := c.Param("id")

	var booking Booking
	if err := db.First(&booking, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
		return
	}
	if booking.Status != StatusPending {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Booking is already %s and cannot be approved", booking.Status)})
		return
	}

	// Guard against double-booking: refuse if another booking for the same
	// facility/date/time was approved in the meantime.
	var approved []Booking
	db.Where("facility_id = ? AND date = ? AND status = ? AND id <> ?",
		booking.FacilityID, booking.Date, StatusApproved, booking.ID).Find(&approved)
	for _, other := range approved {
		overlaps, err := timeRangesOverlap(booking.StartTime, booking.EndTime, other.StartTime, other.EndTime)
		if err != nil {
			log.Printf("approveBooking: could not compare against booking #%d: %v", other.ID, err)
			continue
		}
		if overlaps {
			c.JSON(http.StatusConflict, gin.H{"error": "Another booking for this facility/time has already been approved"})
			return
		}
	}

	booking.Status = StatusApproved
	if err := db.Save(&booking).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update booking"})
		return
	}

	go notify(booking.UserID, "BOOKING_APPROVED", "Booking approved",
		fmt.Sprintf("Your booking #%d has been approved.", booking.ID))

	c.JSON(http.StatusOK, gin.H{
		"message":    "Booking approved successfully",
		"booking_id": booking.ID,
		"status":     booking.Status,
	})
}

// ---------------------------------------------------------------------
// 6. Special business rule — PUT /api/bookings/:id/reject (STAFF/ADMIN)
// ---------------------------------------------------------------------

type RejectBookingRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func rejectBooking(c *gin.Context) {
	id := c.Param("id")

	var booking Booking
	if err := db.First(&booking, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
		return
	}
	// Spec lists 400 (not 409) for this endpoint's error responses, so an
	// already-processed booking is reported as a bad request here.
	if booking.Status != StatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Booking is already %s and cannot be rejected", booking.Status)})
		return
	}

	var req RejectBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}

	booking.Status = StatusRejected
	booking.RejectReason = &req.Reason
	if err := db.Save(&booking).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update booking"})
		return
	}

	go notify(booking.UserID, "BOOKING_REJECTED", "Booking rejected",
		fmt.Sprintf("Your booking #%d was rejected: %s", booking.ID, req.Reason))

	c.JSON(http.StatusOK, gin.H{
		"message":    "Booking rejected successfully",
		"booking_id": booking.ID,
		"status":     booking.Status,
		"reason":     req.Reason,
	})
}

// ---------------------------------------------------------------------
// 5. Delete/Cancel — DELETE /api/bookings/:id (USER, own booking only)
// ---------------------------------------------------------------------

func cancelBooking(c *gin.Context) {
	id := c.Param("id")
	userID := c.MustGet("user_id").(uint)
	role := c.MustGet("role").(string)

	var booking Booking
	if err := db.First(&booking, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found"})
		return
	}

	// Only the booking's own owner may cancel it here (STAFF/ADMIN act on
	// bookings via approve/reject, not this endpoint).
	if role == "USER" && booking.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only cancel your own bookings"})
		return
	}

	if booking.Status == StatusCancelled || booking.Status == StatusRejected {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Booking is already %s and cannot be cancelled", booking.Status)})
		return
	}

	// Soft-delete: keep the row so history/reports stay accurate, matching
	// the CANCELLED value already reserved in the status column's design.
	booking.Status = StatusCancelled
	if err := db.Save(&booking).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel booking"})
		return
	}

	go notify(booking.UserID, "BOOKING_CANCELLED", "Booking cancelled",
		fmt.Sprintf("Booking #%d has been cancelled.", booking.ID))

	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------
// 7. Domain Report — GET /api/reports/bookings/summary (ADMIN)
// ---------------------------------------------------------------------

func reportSummary(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	baseQuery := func() *gorm.DB {
		q := db.Model(&Booking{})
		if startDate != "" {
			q = q.Where("date >= ?", startDate)
		}
		if endDate != "" {
			q = q.Where("date <= ?", endDate)
		}
		return q
	}

	var total, pending, approved, rejected, cancelled int64
	baseQuery().Count(&total)
	baseQuery().Where("status = ?", StatusPending).Count(&pending)
	baseQuery().Where("status = ?", StatusApproved).Count(&approved)
	baseQuery().Where("status = ?", StatusRejected).Count(&rejected)
	baseQuery().Where("status = ?", StatusCancelled).Count(&cancelled)

	c.JSON(http.StatusOK, gin.H{
		"total_bookings": total,
		"pending":        pending,
		"approved":       approved,
		"rejected":       rejected,
		"cancelled":      cancelled,
	})
}

// ---------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------

func main() {
	initDB()

	r := gin.Default()

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "booking-service"})
	})

	// Mounted at "/bookings" so that once Kong strips the "/api/bookings"
	// route prefix (see kong/kong.yml), what's left matches these paths
	// exactly. See TUTORIAL.md for the full request-path walkthrough.
	bookings := r.Group("/bookings")
	bookings.Use(authRequired())
	{
		bookings.POST("", requireRole("USER"), createBooking)
		bookings.GET("", requireRole("USER", "STAFF", "ADMIN"), listBookings)
		bookings.GET("/my", requireRole("USER"), listMyBookings)
		bookings.PUT("/:id/approve", requireRole("STAFF", "ADMIN"), approveBooking)
		bookings.PUT("/:id/reject", requireRole("STAFF", "ADMIN"), rejectBooking)
		bookings.DELETE("/:id", requireRole("USER"), cancelBooking)
	}

	// Mounted at "/reports/bookings" so that once Kong strips
	// "/api/reports/bookings", what's left ("/summary") matches this path.
	reports := r.Group("/reports/bookings")
	reports.Use(authRequired())
	{
		reports.GET("/summary", requireRole("ADMIN"), reportSummary)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}
	log.Printf("Booking Service running on port %s...", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Booking Service failed to start: %v", err)
	}
}
