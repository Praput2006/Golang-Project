package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ส่วนนี้เรียก API ของ Auth & User Service
// token ของ Keycloak มีแค่ keycloak_id (UUID) แต่ตาราง notifications เก็บ users.id (ตัวเลข)
// จึงต้องถาม auth-service โดยส่ง token ของผู้ใช้ต่อไปด้วย

var (
	authServiceURL string
	authHTTPClient = &http.Client{Timeout: 5 * time.Second}

	errAuthUnauthorized = errors.New("auth-service: unauthorized")
	errAuthForbidden    = errors.New("auth-service: forbidden")
	errAuthNotFound     = errors.New("auth-service: not found")
)

// AuthUser คือ field ที่ใช้จาก User ของ auth-service (GET /api/users/me, GET /api/users/:id)
type AuthUser struct {
	ID     int64  `json:"id"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

func connectAuthService(url string) {
	if url == "" {
		panic("AUTH_SERVICE_URL is not set")
	}
	authServiceURL = strings.TrimRight(url, "/")
}

func fetchAuthUser(ctx context.Context, path, authorization string) (*AuthUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authServiceURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authorization)

	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, errAuthUnauthorized
	case http.StatusForbidden:
		return nil, errAuthForbidden
	case http.StatusNotFound:
		return nil, errAuthNotFound
	default:
		return nil, fmt.Errorf("auth-service returned status %d", resp.StatusCode)
	}

	var u AuthUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// getMyAuthUser — GET /api/users/me ของคนที่ login อยู่
func getMyAuthUser(ctx context.Context, authorization string) (*AuthUser, error) {
	return fetchAuthUser(ctx, "/api/users/me", authorization)
}

// getAuthUserByID — GET /api/users/:id (ต้องใช้ token ของ STAFF/ADMIN)
func getAuthUserByID(ctx context.Context, id int64, authorization string) (*AuthUser, error) {
	return fetchAuthUser(ctx, fmt.Sprintf("/api/users/%d", id), authorization)
}
