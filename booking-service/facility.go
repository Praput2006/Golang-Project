package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// FacilityInfo mirrors the fields booking-service actually needs from
// Facility Service's GET /facilities/:id response.
type FacilityInfo struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// errFacilityServiceUnset means FACILITY_SERVICE_URL was not configured.
// Callers should treat this as "validation skipped", not "facility missing" —
// useful while developing booking-service before Facility Service exists.
var errFacilityServiceUnset = errors.New("FACILITY_SERVICE_URL not configured")

// errFacilityNotFound means Facility Service responded 404 for this id.
var errFacilityNotFound = errors.New("facility not found")

// fetchFacility calls Facility Service directly (bypassing Kong, same
// service-to-service pattern order-service uses to call product-service).
func fetchFacility(facilityID uint) (*FacilityInfo, error) {
	baseURL := os.Getenv("FACILITY_SERVICE_URL")
	if baseURL == "" {
		return nil, errFacilityServiceUnset
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/facilities/%d", baseURL, facilityID))
	if err != nil {
		return nil, fmt.Errorf("facility service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errFacilityNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facility service returned status %d", resp.StatusCode)
	}

	var fi FacilityInfo
	if err := json.NewDecoder(resp.Body).Decode(&fi); err != nil {
		return nil, fmt.Errorf("error parsing facility response: %w", err)
	}
	return &fi, nil
}
