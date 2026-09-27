package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Claims is the subset of JWT payload fields this service relies on.
type Claims struct {
	UserID   uint
	Username string
	Role     string
}

// rawClaims mirrors the handful of claim-name variants we might see
// depending on how the Auth service / Keycloak realm ends up configured.
// Coordinate the final claim names with the Auth & User Service owner.
type rawClaims struct {
	Sub      string      `json:"sub"`
	UserID   json.Number `json:"user_id"`
	ID       json.Number `json:"id"`
	Username string      `json:"username"`
	PrefName string      `json:"preferred_username"`
	Role     string      `json:"role"`
}

// decodeJWTPayload extracts and base64-decodes the middle (payload) segment
// of a JWT WITHOUT verifying its signature.
//
// This is safe to do here ONLY because Kong's `jwt` plugin sits in front of
// this service (see kong/kong.yml) and already rejects requests whose token
// has an invalid signature or an expired `exp` claim before they ever reach
// booking-service. If this service is ever exposed without Kong in front of
// it, this function must NOT be relied on for authorization.
func decodeJWTPayload(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}
	payload := parts[1]
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	return base64.URLEncoding.DecodeString(payload)
}

// ParseClaims decodes a raw bearer token string into a Claims struct.
// It returns an error if the token is malformed or has no role claim.
func ParseClaims(token string) (*Claims, error) {
	data, err := decodeJWTPayload(token)
	if err != nil {
		return nil, err
	}
	var rc rawClaims
	if err := json.Unmarshal(data, &rc); err != nil {
		return nil, err
	}

	c := &Claims{Role: rc.Role}

	switch {
	case rc.Username != "":
		c.Username = rc.Username
	case rc.PrefName != "":
		c.Username = rc.PrefName
	default:
		c.Username = rc.Sub
	}

	if id, err := strconv.ParseUint(rc.UserID.String(), 10, 64); err == nil {
		c.UserID = uint(id)
	} else if id, err := strconv.ParseUint(rc.ID.String(), 10, 64); err == nil {
		c.UserID = uint(id)
	} else if id, err := strconv.ParseUint(rc.Sub, 10, 64); err == nil {
		c.UserID = uint(id)
	}

	if c.Role == "" {
		return nil, errors.New("token missing role claim")
	}
	return c, nil
}
