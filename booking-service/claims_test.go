package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func makeTestToken(claims map[string]interface{}) string {
	header := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(claims)
	payload := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(body)
	// The signature is irrelevant for these tests: Kong has already verified
	// it before this service ever sees the token, so ParseClaims never
	// checks it (see the comment on decodeJWTPayload).
	return header + "." + payload + ".fakesignature"
}

func TestParseClaimsRole(t *testing.T) {
	token := makeTestToken(map[string]interface{}{
		"sub":      "42",
		"username": "phasakorn",
		"role":     "ADMIN",
	})
	c, err := ParseClaims(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Role != "ADMIN" {
		t.Errorf("Role = %q, want ADMIN", c.Role)
	}
	if c.UserID != 42 {
		t.Errorf("UserID = %d, want 42", c.UserID)
	}
	if c.Username != "phasakorn" {
		t.Errorf("Username = %q, want phasakorn", c.Username)
	}
}

func TestParseClaimsFallsBackToUserIDClaim(t *testing.T) {
	token := makeTestToken(map[string]interface{}{
		"sub":     "some-keycloak-uuid",
		"user_id": 7,
		"role":    "STAFF",
	})
	c, err := ParseClaims(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.UserID != 7 {
		t.Errorf("UserID = %d, want 7", c.UserID)
	}
}

func TestParseClaimsMissingRole(t *testing.T) {
	token := makeTestToken(map[string]interface{}{"sub": "1"})
	if _, err := ParseClaims(token); err == nil {
		t.Error("expected error for token missing role claim")
	}
}

func TestParseClaimsMalformed(t *testing.T) {
	if _, err := ParseClaims("not-a-jwt"); err == nil {
		t.Error("expected error for malformed token")
	}
}
