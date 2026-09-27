package repository

import (
	"context"
	"errors"
	"time"

	"facility-service/internal/model"

	"gorm.io/gorm"
)

type ListFilter struct {
	Query       string
	Type        string
	Status      string
	MinCapacity int
	Offset      int
	Limit       int
}

type FacilityUsageRow struct {
	FacilityID     int64
	Name           string
	Type           string
	Capacity       int
	Status         string
	SlotCount      int64
	AvailableHours float64
}

type FacilityRepository interface {
	Create(ctx context.Context, f *model.Facility) error
	List(ctx context.Context, filter ListFilter) ([]model.Facility, int64, error)
	FindByID(ctx context.Context, id int64) (*model.Facility, error) // ไม่พบ → (nil, nil)
	ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error)
	Update(ctx context.Context, f *model.Facility) error
	Delete(ctx context.Context, id int64) error

	CountAvailabilityFrom(ctx context.Context, facilityID int64, from time.Time) (int64, error)
	FindAvailabilityFrom(ctx context.Context, facilityID int64, from time.Time, limit int) ([]model.FacilityAvailability, error)
	HasOverlap(ctx context.Context, facilityID int64, date time.Time, start, end string) (bool, error)
	CreateAvailability(ctx context.Context, a *model.FacilityAvailability) error

	UsageByFacility(ctx context.Context, from, to time.Time) ([]FacilityUsageRow, error)
}

type gormFacilityRepository struct {
	db *gorm.DB
}

func NewFacilityRepository(db *gorm.DB) FacilityRepository {
	return &gormFacilityRepository{db: db}
}

func (r *gormFacilityRepository) Create(ctx context.Context, f *model.Facility) error {
	return r.db.WithContext(ctx).Create(f).Error
}

func (r *gormFacilityRepository) List(ctx context.Context, f ListFilter) ([]model.Facility, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Facility{})
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("(name ILIKE ? OR description ILIKE ?)", like, like)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.MinCapacity > 0 {
		q = q.Where("capacity >= ?", f.MinCapacity)
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []model.Facility
	err := q.Session(&gorm.Session{}).
		Order("id ASC").Offset(f.Offset).Limit(f.Limit).
		Find(&items).Error
	return items, total, err
}

func (r *gormFacilityRepository) FindByID(ctx context.Context, id int64) (*model.Facility, error) {
	var f model.Facility
	err := r.db.WithContext(ctx).First(&f, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *gormFacilityRepository) ExistsByName(ctx context.Context, name string, excludeID int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.Facility{}).Where("LOWER(name) = LOWER(?)", name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var n int64
	err := q.Count(&n).Error
	return n > 0, err
}

func (r *gormFacilityRepository) Update(ctx context.Context, f *model.Facility) error {
	return r.db.WithContext(ctx).Save(f).Error
}

func (r *gormFacilityRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("facility_id = ?", id).Delete(&model.FacilityAvailability{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Facility{}, id).Error
	})
}

func (r *gormFacilityRepository) CountAvailabilityFrom(ctx context.Context, facilityID int64, from time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.FacilityAvailability{}).
		Where("facility_id = ? AND date >= ?", facilityID, from).
		Count(&n).Error
	return n, err
}

func (r *gormFacilityRepository) FindAvailabilityFrom(ctx context.Context, facilityID int64, from time.Time, limit int) ([]model.FacilityAvailability, error) {
	var slots []model.FacilityAvailability
	err := r.db.WithContext(ctx).
		Where("facility_id = ? AND date >= ?", facilityID, from).
		Order("date ASC, start_time ASC").
		Limit(limit).
		Find(&slots).Error
	return slots, err
}

// ช่วงเวลาทับกันเมื่อ existing.start < new.end AND existing.end > new.start
// (ช่วงที่ติดกันพอดี เช่น 09:00-11:00 กับ 11:00-12:00 ถือว่าไม่ทับ)
func (r *gormFacilityRepository) HasOverlap(ctx context.Context, facilityID int64, date time.Time, start, end string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.FacilityAvailability{}).
		Where("facility_id = ? AND date = ? AND start_time < ? AND end_time > ?", facilityID, date, end, start).
		Count(&n).Error
	return n > 0, err
}

func (r *gormFacilityRepository) CreateAvailability(ctx context.Context, a *model.FacilityAvailability) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormFacilityRepository) UsageByFacility(ctx context.Context, from, to time.Time) ([]FacilityUsageRow, error) {
	var rows []FacilityUsageRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT f.id AS facility_id, f.name, f.type, f.capacity, f.status,
		       COUNT(a.id) AS slot_count,
		       COALESCE(SUM(EXTRACT(EPOCH FROM (a.end_time - a.start_time))) / 3600.0, 0) AS available_hours
		FROM facilities f
		LEFT JOIN facility_availability a
		       ON a.facility_id = f.id AND a.date BETWEEN ? AND ?
		GROUP BY f.id
		ORDER BY available_hours DESC, f.id ASC`, from, to).
		Scan(&rows).Error
	return rows, err
}
