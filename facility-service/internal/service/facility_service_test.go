package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"facility-service/internal/dto"
	"facility-service/internal/model"
	"facility-service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- in-memory fake repository ----------

type fakeRepo struct {
	facilities map[int64]*model.Facility
	slots      []model.FacilityAvailability
	nextID     int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{facilities: map[int64]*model.Facility{}} }

func (r *fakeRepo) id() int64 { r.nextID++; return r.nextID }

func (r *fakeRepo) Create(_ context.Context, f *model.Facility) error {
	f.ID = r.id()
	cp := *f
	r.facilities[f.ID] = &cp
	return nil
}
func (r *fakeRepo) List(_ context.Context, _ repository.ListFilter) ([]model.Facility, int64, error) {
	var out []model.Facility
	for _, f := range r.facilities {
		out = append(out, *f)
	}
	return out, int64(len(out)), nil
}
func (r *fakeRepo) FindByID(_ context.Context, id int64) (*model.Facility, error) {
	if f, ok := r.facilities[id]; ok {
		cp := *f
		return &cp, nil
	}
	return nil, nil
}
func (r *fakeRepo) ExistsByName(_ context.Context, name string, excludeID int64) (bool, error) {
	for _, f := range r.facilities {
		if f.ID != excludeID && strings.EqualFold(f.Name, name) {
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeRepo) Update(_ context.Context, f *model.Facility) error {
	cp := *f
	r.facilities[f.ID] = &cp
	return nil
}
func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	delete(r.facilities, id)
	kept := r.slots[:0]
	for _, s := range r.slots {
		if s.FacilityID != id {
			kept = append(kept, s)
		}
	}
	r.slots = kept
	return nil
}
func (r *fakeRepo) CountAvailabilityFrom(_ context.Context, id int64, from time.Time) (int64, error) {
	var n int64
	for _, s := range r.slots {
		if s.FacilityID == id && !s.Date.Before(from) {
			n++
		}
	}
	return n, nil
}
func (r *fakeRepo) FindAvailabilityFrom(_ context.Context, id int64, from time.Time, _ int) ([]model.FacilityAvailability, error) {
	var out []model.FacilityAvailability
	for _, s := range r.slots {
		if s.FacilityID == id && !s.Date.Before(from) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (r *fakeRepo) HasOverlap(_ context.Context, id int64, date time.Time, start, end string) (bool, error) {
	for _, s := range r.slots {
		if s.FacilityID == id && s.Date.Equal(date) && s.StartTime < end && s.EndTime > start {
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeRepo) CreateAvailability(_ context.Context, a *model.FacilityAvailability) error {
	a.ID = r.id()
	r.slots = append(r.slots, *a)
	return nil
}
func (r *fakeRepo) UsageByFacility(_ context.Context, _, _ time.Time) ([]repository.FacilityUsageRow, error) {
	return []repository.FacilityUsageRow{
		{FacilityID: 1, Name: "A", Type: "LAB", Capacity: 20, Status: "AVAILABLE", SlotCount: 2, AvailableHours: 5},
		{FacilityID: 2, Name: "B", Type: "LAB", Capacity: 40, Status: "UNAVAILABLE", SlotCount: 1, AvailableHours: 1.5},
	}, nil
}

// ---------- helpers ----------

// "ตอนนี้" ในเทสต์ = 27 ก.ย. 2026 เวลา 10:00
var fixedNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.Local)

func newTestService(t *testing.T) (*facilityService, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	return &facilityService{repo: repo, now: func() time.Time { return fixedNow }}, repo
}

func seedFacility(t *testing.T, s *facilityService, name, status string) int64 {
	t.Helper()
	res, err := s.Create(context.Background(), dto.CreateFacilityRequest{
		Name: name, Type: "MEETING_ROOM", Capacity: 10, Status: status,
	})
	require.NoError(t, err)
	return res.ID
}

func slot(date, start, end string) dto.CreateAvailabilityRequest {
	return dto.CreateAvailabilityRequest{Date: date, StartTime: start, EndTime: end}
}

// ---------- Create / Update ----------

func TestCreate_DefaultStatusAvailable(t *testing.T) {
	s, _ := newTestService(t)
	res, err := s.Create(context.Background(), dto.CreateFacilityRequest{Name: "Room 1", Type: "LAB", Capacity: 5})
	require.NoError(t, err)
	assert.Equal(t, model.StatusAvailable, res.Status)
}

func TestCreate_DuplicateNameConflict(t *testing.T) {
	s, _ := newTestService(t)
	seedFacility(t, s, "Room 1", "")
	_, err := s.Create(context.Background(), dto.CreateFacilityRequest{Name: "room 1", Type: "LAB", Capacity: 5})
	assert.ErrorIs(t, err, ErrConflict)
}

func TestUpdate_PartialFields(t *testing.T) {
	s, _ := newTestService(t)
	id := seedFacility(t, s, "Room 1", "")
	cap := 99
	res, err := s.Update(context.Background(), id, dto.UpdateFacilityRequest{Capacity: &cap})
	require.NoError(t, err)
	assert.Equal(t, 99, res.Capacity)
	assert.Equal(t, "Room 1", res.Name) // field อื่นไม่เปลี่ยน
}

func TestUpdate_NotFound(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.Update(context.Background(), 404, dto.UpdateFacilityRequest{})
	assert.ErrorIs(t, err, ErrNotFound)
}

// ---------- Special: AddAvailability ----------

func TestAddAvailability_Success(t *testing.T) {
	s, _ := newTestService(t)
	id := seedFacility(t, s, "Room 1", "")
	res, err := s.AddAvailability(context.Background(), id, slot("2026-09-28", "09:00", "11:00"))
	require.NoError(t, err)
	assert.Equal(t, "2026-09-28", res.Date)
	assert.Equal(t, "09:00", res.StartTime)
}

func TestAddAvailability_Rules(t *testing.T) {
	cases := []struct {
		name    string
		req     dto.CreateAvailabilityRequest
		wantErr error
	}{
		{"past date", slot("2026-09-26", "09:00", "10:00"), ErrBadRequest},
		{"today but already started", slot("2026-09-27", "09:30", "11:00"), ErrBadRequest},
		{"end before start", slot("2026-09-28", "12:00", "10:00"), ErrBadRequest},
		{"end equals start", slot("2026-09-28", "10:00", "10:00"), ErrBadRequest},
		{"today later is fine", slot("2026-09-27", "13:00", "14:00"), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestService(t)
			id := seedFacility(t, s, "Room", "")
			_, err := s.AddAvailability(context.Background(), id, tc.req)
			if tc.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestAddAvailability_OverlapConflict(t *testing.T) {
	s, _ := newTestService(t)
	id := seedFacility(t, s, "Room 1", "")
	ctx := context.Background()

	_, err := s.AddAvailability(ctx, id, slot("2026-09-28", "09:00", "11:00"))
	require.NoError(t, err)

	_, err = s.AddAvailability(ctx, id, slot("2026-09-28", "10:00", "12:00"))
	assert.ErrorIs(t, err, ErrConflict, "ทับช่วง 10:00-11:00")

	_, err = s.AddAvailability(ctx, id, slot("2026-09-28", "11:00", "12:00"))
	assert.NoError(t, err, "ต่อกันพอดีไม่ถือว่าทับ")

	_, err = s.AddAvailability(ctx, id, slot("2026-09-29", "09:00", "11:00"))
	assert.NoError(t, err, "คนละวันไม่ทับ")
}

func TestAddAvailability_UnavailableFacility(t *testing.T) {
	s, _ := newTestService(t)
	id := seedFacility(t, s, "Room 1", model.StatusUnavailable)
	_, err := s.AddAvailability(context.Background(), id, slot("2026-09-28", "09:00", "11:00"))
	assert.ErrorIs(t, err, ErrConflict)
}

func TestAddAvailability_FacilityNotFound(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.AddAvailability(context.Background(), 404, slot("2026-09-28", "09:00", "11:00"))
	assert.ErrorIs(t, err, ErrNotFound)
}

// ---------- Delete ----------

func TestDelete_ConflictWhenUpcomingSlots_ThenForce(t *testing.T) {
	s, repo := newTestService(t)
	id := seedFacility(t, s, "Room 1", "")
	ctx := context.Background()
	_, err := s.AddAvailability(ctx, id, slot("2026-10-01", "09:00", "10:00"))
	require.NoError(t, err)

	assert.ErrorIs(t, s.Delete(ctx, id, false), ErrConflict)

	require.NoError(t, s.Delete(ctx, id, true))
	assert.Empty(t, repo.facilities)
	assert.Empty(t, repo.slots)
}

func TestDelete_NoSlotsOK(t *testing.T) {
	s, _ := newTestService(t)
	id := seedFacility(t, s, "Room 1", "")
	assert.NoError(t, s.Delete(context.Background(), id, false))
}

// ---------- Read one / Report ----------

func TestGetByID_IncludesUpcomingSlots(t *testing.T) {
	s, _ := newTestService(t)
	id := seedFacility(t, s, "Room 1", "")
	_, err := s.AddAvailability(context.Background(), id, slot("2026-09-28", "09:00", "11:00"))
	require.NoError(t, err)

	res, err := s.GetByID(context.Background(), id)
	require.NoError(t, err)
	assert.Len(t, res.UpcomingAvailability, 1)
}

func TestUsageReport_Aggregates(t *testing.T) {
	s, _ := newTestService(t)
	rep, err := s.UsageReport(context.Background(), dto.UsageReportQuery{})
	require.NoError(t, err)
	assert.Equal(t, "2026-09-01", rep.From)
	assert.Equal(t, "2026-09-30", rep.To)
	assert.Equal(t, 2, rep.TotalFacilities)
	assert.Equal(t, 60, rep.TotalCapacity)
	assert.Equal(t, int64(3), rep.TotalSlots)
	assert.Equal(t, 6.5, rep.TotalAvailableHours)
	assert.Equal(t, 2, rep.ByType["LAB"])
	assert.Equal(t, 1, rep.ByStatus["UNAVAILABLE"])
}

func TestUsageReport_InvalidRange(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.UsageReport(context.Background(), dto.UsageReportQuery{From: "2026-10-10", To: "2026-10-01"})
	assert.ErrorIs(t, err, ErrBadRequest)
}
