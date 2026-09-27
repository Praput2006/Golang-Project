package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"facility-service/internal/dto"
	"facility-service/internal/model"
	"facility-service/internal/repository"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrBadRequest = errors.New("bad request")
)

type FacilityService interface {
	Create(ctx context.Context, req dto.CreateFacilityRequest) (*dto.FacilityResponse, error)
	List(ctx context.Context, q dto.ListFacilityQuery) (*dto.PagedResponse[dto.FacilityResponse], error)
	GetByID(ctx context.Context, id int64) (*dto.FacilityDetailResponse, error)
	Update(ctx context.Context, id int64, req dto.UpdateFacilityRequest) (*dto.FacilityResponse, error)
	Delete(ctx context.Context, id int64, force bool) error
	AddAvailability(ctx context.Context, id int64, req dto.CreateAvailabilityRequest) (*dto.AvailabilityResponse, error)
	UsageReport(ctx context.Context, q dto.UsageReportQuery) (*dto.UsageReportResponse, error)
}

type facilityService struct {
	repo repository.FacilityRepository
	now  func() time.Time // แยกออกมาเพื่อให้ unit test กำหนดเวลาได้
}

func NewFacilityService(repo repository.FacilityRepository) FacilityService {
	return &facilityService{repo: repo, now: time.Now}
}

// วันนี้ในรูปแบบเดียวกับค่า DATE ที่อ่านจาก DB (00:00 UTC)
func (s *facilityService) today() time.Time {
	n := s.now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// 1. Create
func (s *facilityService) Create(ctx context.Context, req dto.CreateFacilityRequest) (*dto.FacilityResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrBadRequest)
	}
	exists, err := s.repo.ExistsByName(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("%w: facility name %q already exists", ErrConflict, name)
	}

	status := req.Status
	if status == "" {
		status = model.StatusAvailable
	}
	f := &model.Facility{
		Name: name, Type: req.Type, Capacity: req.Capacity,
		Description: req.Description, Status: status,
	}
	if err := s.repo.Create(ctx, f); err != nil {
		return nil, err
	}
	res := dto.ToFacilityResponse(f)
	return &res, nil
}

// 2. List / Search
func (s *facilityService) List(ctx context.Context, q dto.ListFacilityQuery) (*dto.PagedResponse[dto.FacilityResponse], error) {
	page, limit := q.Page, q.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	items, total, err := s.repo.List(ctx, repository.ListFilter{
		Query:       strings.TrimSpace(q.Q),
		Type:        q.Type,
		Status:      q.Status,
		MinCapacity: q.MinCapacity,
		Offset:      (page - 1) * limit,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}

	data := make([]dto.FacilityResponse, 0, len(items))
	for i := range items {
		data = append(data, dto.ToFacilityResponse(&items[i]))
	}
	return &dto.PagedResponse[dto.FacilityResponse]{
		Data: data, Page: page, Limit: limit, Total: total,
		TotalPages: int((total + int64(limit) - 1) / int64(limit)),
	}, nil
}

// 3. Read one (แนบช่วงเวลาที่เปิดให้จองที่ยังไม่ผ่านไปด้วย)
func (s *facilityService) GetByID(ctx context.Context, id int64) (*dto.FacilityDetailResponse, error) {
	f, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: facility %d", ErrNotFound, id)
	}

	slots, err := s.repo.FindAvailabilityFrom(ctx, id, s.today(), 50)
	if err != nil {
		return nil, err
	}
	upcoming := make([]dto.AvailabilityResponse, 0, len(slots))
	for i := range slots {
		upcoming = append(upcoming, dto.ToAvailabilityResponse(&slots[i]))
	}
	return &dto.FacilityDetailResponse{
		FacilityResponse:     dto.ToFacilityResponse(f),
		UpcomingAvailability: upcoming,
	}, nil
}

// 4. Update (partial)
func (s *facilityService) Update(ctx context.Context, id int64, req dto.UpdateFacilityRequest) (*dto.FacilityResponse, error) {
	f, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: facility %d", ErrNotFound, id)
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrBadRequest)
		}
		exists, err := s.repo.ExistsByName(ctx, name, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("%w: facility name %q already exists", ErrConflict, name)
		}
		f.Name = name
	}
	if req.Type != nil {
		f.Type = *req.Type
	}
	if req.Capacity != nil {
		f.Capacity = *req.Capacity
	}
	if req.Description != nil {
		f.Description = *req.Description
	}
	if req.Status != nil {
		f.Status = *req.Status
	}

	if err := s.repo.Update(ctx, f); err != nil {
		return nil, err
	}
	res := dto.ToFacilityResponse(f)
	return &res, nil
}

// 5. Delete — conflict ถ้ายังมีช่วงเวลาเปิดจองในอนาคต เว้นแต่ส่ง force=true
func (s *facilityService) Delete(ctx context.Context, id int64, force bool) error {
	f, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if f == nil {
		return fmt.Errorf("%w: facility %d", ErrNotFound, id)
	}

	if !force {
		n, err := s.repo.CountAvailabilityFrom(ctx, id, s.today())
		if err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: facility has %d upcoming availability slot(s); retry with ?force=true to delete them as well", ErrConflict, n)
		}
	}
	return s.repo.Delete(ctx, id)
}

// 6. Special business rule — กำหนดวันเวลาที่เปิดให้จอง
func (s *facilityService) AddAvailability(ctx context.Context, id int64, req dto.CreateAvailabilityRequest) (*dto.AvailabilityResponse, error) {
	f, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: facility %d", ErrNotFound, id)
	}
	if f.Status != model.StatusAvailable {
		return nil, fmt.Errorf("%w: facility is %s, cannot open for booking", ErrConflict, f.Status)
	}

	date, err := time.Parse(dto.DateLayout, req.Date)
	if err != nil {
		return nil, fmt.Errorf("%w: date must be YYYY-MM-DD", ErrBadRequest)
	}
	start, err := time.Parse(dto.ClockLayout, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("%w: start_time must be HH:MM", ErrBadRequest)
	}
	end, err := time.Parse(dto.ClockLayout, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("%w: end_time must be HH:MM", ErrBadRequest)
	}
	if !start.Before(end) {
		return nil, fmt.Errorf("%w: start_time must be before end_time", ErrBadRequest)
	}

	today := s.today()
	if date.Before(today) {
		return nil, fmt.Errorf("%w: cannot open availability for a past date", ErrBadRequest)
	}
	if date.Equal(today) {
		n := s.now()
		nowClock := time.Date(0, 1, 1, n.Hour(), n.Minute(), 0, 0, time.UTC)
		if !start.After(nowClock) {
			return nil, fmt.Errorf("%w: start_time has already passed today", ErrBadRequest)
		}
	}

	overlap, err := s.repo.HasOverlap(ctx, id, date, req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}
	if overlap {
		return nil, fmt.Errorf("%w: time slot overlaps an existing availability on %s", ErrConflict, req.Date)
	}

	a := &model.FacilityAvailability{
		FacilityID: id, Date: date,
		StartTime: req.StartTime, EndTime: req.EndTime,
	}
	if err := s.repo.CreateAvailability(ctx, a); err != nil {
		return nil, err
	}
	res := dto.ToAvailabilityResponse(a)
	return &res, nil
}

// 7. Domain report — สถิติห้อง + ชั่วโมงที่เปิดให้จองในช่วง from..to (ค่าเริ่มต้น = เดือนปัจจุบัน)
func (s *facilityService) UsageReport(ctx context.Context, q dto.UsageReportQuery) (*dto.UsageReportResponse, error) {
	first := s.today().AddDate(0, 0, -s.today().Day()+1)
	from, to := first, first.AddDate(0, 1, -1)

	var err error
	if q.From != "" {
		if from, err = time.Parse(dto.DateLayout, q.From); err != nil {
			return nil, fmt.Errorf("%w: from must be YYYY-MM-DD", ErrBadRequest)
		}
	}
	if q.To != "" {
		if to, err = time.Parse(dto.DateLayout, q.To); err != nil {
			return nil, fmt.Errorf("%w: to must be YYYY-MM-DD", ErrBadRequest)
		}
	}
	if to.Before(from) {
		return nil, fmt.Errorf("%w: to must not be before from", ErrBadRequest)
	}

	rows, err := s.repo.UsageByFacility(ctx, from, to)
	if err != nil {
		return nil, err
	}

	rep := &dto.UsageReportResponse{
		From: from.Format(dto.DateLayout), To: to.Format(dto.DateLayout),
		ByType: map[string]int{}, ByStatus: map[string]int{},
		Facilities: make([]dto.FacilityUsage, 0, len(rows)),
	}
	for _, r := range rows {
		hours := round2(r.AvailableHours)
		rep.TotalFacilities++
		rep.TotalCapacity += r.Capacity
		rep.TotalSlots += r.SlotCount
		rep.TotalAvailableHours += hours
		rep.ByType[r.Type]++
		rep.ByStatus[r.Status]++
		rep.Facilities = append(rep.Facilities, dto.FacilityUsage{
			FacilityID: r.FacilityID, Name: r.Name, Type: r.Type,
			Capacity: r.Capacity, Status: r.Status,
			SlotCount: r.SlotCount, AvailableHours: hours,
		})
	}
	rep.TotalAvailableHours = round2(rep.TotalAvailableHours)
	return rep, nil
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
