package model

import "time"

const (
	StatusAvailable   = "AVAILABLE"
	StatusUnavailable = "UNAVAILABLE"
)

// facilities (facility_db)
type Facility struct {
	ID             int64  `gorm:"primaryKey"`
	Name           string `gorm:"type:varchar(100);not null"`
	Type           string `gorm:"type:varchar(30);not null;index"`
	Capacity       int    `gorm:"not null"`
	Description    string `gorm:"type:text"`
	Status         string `gorm:"type:varchar(20);not null;default:AVAILABLE;index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Availabilities []FacilityAvailability `gorm:"foreignKey:FacilityID;constraint:OnDelete:CASCADE"`
}

func (Facility) TableName() string { return "facilities" }

// facility_availability (FK → facilities.id ภายใน DB เดียวกัน)
type FacilityAvailability struct {
	ID         int64     `gorm:"primaryKey"`
	FacilityID int64     `gorm:"not null;index:idx_avail_facility_date"`
	Date       time.Time `gorm:"type:date;not null;index:idx_avail_facility_date"`
	StartTime  string    `gorm:"type:time without time zone;not null"`
	EndTime    string    `gorm:"type:time without time zone;not null"`
	CreatedAt  time.Time
}

func (FacilityAvailability) TableName() string { return "facility_availability" }
