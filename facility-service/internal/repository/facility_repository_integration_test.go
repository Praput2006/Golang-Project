//go:build integration

//	รันด้วย: TEST_DATABASE_URL="host=localhost user=postgres password=postgres dbname=facility_db_test port=5432 sslmode=disable" \
//	         go test -tags=integration ./internal/repository/...
//
// (ใช้ DB แยกสำหรับเทสต์ เพราะจะล้างตารางทิ้ง)
package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"facility-service/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupDB(t *testing.T) (*gorm.DB, FacilityRepository) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&model.FacilityAvailability{}, &model.Facility{}))
	require.NoError(t, db.AutoMigrate(&model.Facility{}, &model.FacilityAvailability{}))
	return db, NewFacilityRepository(db)
}

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func TestRepository_EndToEnd(t *testing.T) {
	_, repo := setupDB(t)
	ctx := context.Background()

	a := &model.Facility{Name: "Room A", Type: "LAB", Capacity: 20, Status: "AVAILABLE", Description: "projector"}
	b := &model.Facility{Name: "Hall B", Type: "HALL", Capacity: 200, Status: "UNAVAILABLE"}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))

	// List + filter + search + pagination
	items, total, err := repo.List(ctx, ListFilter{Query: "project", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Room A", items[0].Name)

	items, total, err = repo.List(ctx, ListFilter{MinCapacity: 100, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Hall B", items[0].Name)

	items, total, err = repo.List(ctx, ListFilter{Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 1)

	// ExistsByName (case-insensitive, exclude self)
	ok, err := repo.ExistsByName(ctx, "room a", 0)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, _ = repo.ExistsByName(ctx, "room a", a.ID)
	assert.False(t, ok)

	// Availability + overlap
	require.NoError(t, repo.CreateAvailability(ctx, &model.FacilityAvailability{FacilityID: a.ID, Date: d("2026-10-01"), StartTime: "09:00", EndTime: "11:00"}))
	require.NoError(t, repo.CreateAvailability(ctx, &model.FacilityAvailability{FacilityID: a.ID, Date: d("2026-10-02"), StartTime: "13:00", EndTime: "14:30"}))

	over, err := repo.HasOverlap(ctx, a.ID, d("2026-10-01"), "10:00", "12:00")
	require.NoError(t, err)
	assert.True(t, over)
	over, _ = repo.HasOverlap(ctx, a.ID, d("2026-10-01"), "11:00", "12:00")
	assert.False(t, over)

	slots, err := repo.FindAvailabilityFrom(ctx, a.ID, d("2026-10-01"), 10)
	require.NoError(t, err)
	require.Len(t, slots, 2)
	assert.Equal(t, "2026-10-01", slots[0].Date.Format("2006-01-02"))
	assert.Equal(t, "09:00", slots[0].StartTime[:5])

	n, err := repo.CountAvailabilityFrom(ctx, a.ID, d("2026-10-02"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Report
	rows, err := repo.UsageByFacility(ctx, d("2026-10-01"), d("2026-10-31"))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, a.ID, rows[0].FacilityID)
	assert.Equal(t, int64(2), rows[0].SlotCount)
	assert.InDelta(t, 3.5, rows[0].AvailableHours, 0.001)
	assert.Equal(t, int64(0), rows[1].SlotCount)

	// Update + Delete (cascade availability)
	a.Capacity = 25
	require.NoError(t, repo.Update(ctx, a))
	got, err := repo.FindByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, 25, got.Capacity)

	require.NoError(t, repo.Delete(ctx, a.ID))
	got, err = repo.FindByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	n, _ = repo.CountAvailabilityFrom(ctx, a.ID, d("2000-01-01"))
	assert.Equal(t, int64(0), n)
}
