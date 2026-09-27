package main

import "testing"

func TestTimeRangesOverlap(t *testing.T) {
	cases := []struct {
		name                       string
		aStart, aEnd, bStart, bEnd string
		want                       bool
	}{
		{"identical ranges overlap", "09:00", "11:00", "09:00", "11:00", true},
		{"partial overlap", "09:00", "11:00", "10:00", "12:00", true},
		{"back-to-back does not overlap", "09:00", "11:00", "11:00", "12:00", false},
		{"fully disjoint", "09:00", "10:00", "14:00", "15:00", false},
		{"b fully inside a", "09:00", "17:00", "10:00", "11:00", true},
		{"a fully inside b", "10:00", "11:00", "09:00", "17:00", true},
		{"HH:MM:SS as returned by Postgres round-trip", "09:00", "11:00", "10:00:00", "12:00:00", true},
		{"HH:MM:SS no overlap", "09:00", "10:00", "14:00:00", "15:00:00", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := timeRangesOverlap(tc.aStart, tc.aEnd, tc.bStart, tc.bEnd)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("timeRangesOverlap(%s-%s, %s-%s) = %v, want %v",
					tc.aStart, tc.aEnd, tc.bStart, tc.bEnd, got, tc.want)
			}
		})
	}
}

func TestTimeRangesOverlapInvalidInput(t *testing.T) {
	if _, err := timeRangesOverlap("11:00", "09:00", "09:00", "10:00"); err == nil {
		t.Error("expected error when start_time is after end_time")
	}
	if _, err := timeRangesOverlap("bad", "11:00", "09:00", "10:00"); err == nil {
		t.Error("expected error for malformed start_time")
	}
}
