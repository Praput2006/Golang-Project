package main

import (
	"fmt"
	"time"
)

// parseTimeOfDay parses a time-of-day string into a time.Time so two times
// can be compared. It accepts both "HH:MM" (what clients send in requests)
// and "HH:MM:SS" (what Postgres round-trips a `time` column back as via
// GORM/pgx) — confirmed against a real Postgres instance while building
// this service; comparisons silently went wrong without this fallback.
func parseTimeOfDay(s string) (time.Time, error) {
	if t, err := time.Parse("15:04", s); err == nil {
		return t, nil
	}
	return time.Parse("15:04:05", s)
}

// parseRange parses and validates a start/end pair, returning an error if
// either value is malformed or start is not strictly before end.
func parseRange(start, end string) (time.Time, time.Time, error) {
	s, err := parseTimeOfDay(start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time %q: %w", start, err)
	}
	e, err := parseTimeOfDay(end)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time %q: %w", end, err)
	}
	if !s.Before(e) {
		return time.Time{}, time.Time{}, fmt.Errorf("start_time must be before end_time")
	}
	return s, e, nil
}

// normalizeDate returns a date string in canonical "YYYY-MM-DD" form.
//
// GORM/pgx round-trips a Postgres `date` column back into a Go `string`
// field as a full RFC3339 timestamp (e.g. "2026-10-10T00:00:00Z"), not the
// plain "2026-10-10" the API contract promises — confirmed against a real
// Postgres instance while building this service. Call this on any Date
// value that came from a DB read before putting it in a JSON response.
// Freshly-created values (not yet re-read from the DB) are already clean,
// but normalizing them too is harmless and keeps every call site the same.
func normalizeDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2006-01-02")
	}
	return s
}

// timeRangesOverlap reports whether the half-open interval [startA, endA)
// overlaps [startB, endB). All four strings must be "HH:MM" 24-hour times.
// Back-to-back ranges (one ends exactly when the other starts) do NOT count
// as overlapping, matching how real room bookings work.
func timeRangesOverlap(startA, endA, startB, endB string) (bool, error) {
	sa, ea, err := parseRange(startA, endA)
	if err != nil {
		return false, err
	}
	sb, eb, err := parseRange(startB, endB)
	if err != nil {
		return false, err
	}
	// Standard interval overlap test: A starts before B ends AND B starts before A ends.
	return sa.Before(eb) && sb.Before(ea), nil
}
