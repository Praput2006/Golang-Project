package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/Nerzal/gocloak/v13"
)

// ส่วนนี้คุยกับ Keycloak Admin API ผ่าน library gocloak
// รหัสผ่านเก็บที่ Keycloak เท่านั้น auth-service ไม่เก็บรหัสผ่านเอง

var (
	kc                   *gocloak.GoCloak
	keycloakRealm        string
	keycloakClientID     string
	keycloakClientSecret string
)

func connectKeycloak(url, realm, clientID, clientSecret string) {
	kc = gocloak.NewClient(url)
	keycloakRealm = realm
	keycloakClientID = clientID
	keycloakClientSecret = clientSecret
}

// adminToken ขอ token ของ service account "auth-service" เพื่อใช้เรียก Admin API
func adminToken(ctx context.Context) (string, error) {
	token, err := kc.LoginClient(ctx, keycloakClientID, keycloakClientSecret, keycloakRealm)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func isKeycloakConflict(err error) bool {
	var apiErr *gocloak.APIError
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusConflict
}

func createKeycloakUser(ctx context.Context, req RegisterRequest) (string, error) {
	token, err := adminToken(ctx)
	if err != nil {
		return "", err
	}

	user := gocloak.User{
		Username:  gocloak.StringP(req.Username),
		Email:     gocloak.StringP(req.Email),
		FirstName: gocloak.StringP(req.FirstName),
		LastName:  gocloak.StringP(req.LastName),
		Enabled:   gocloak.BoolP(true),
		Credentials: &[]gocloak.CredentialRepresentation{{
			Type:      gocloak.StringP("password"),
			Value:     gocloak.StringP(req.Password),
			Temporary: gocloak.BoolP(false),
		}},
	}
	return kc.CreateUser(ctx, token, keycloakRealm, user)
}

func updateKeycloakUser(ctx context.Context, keycloakID string, req UpdateUserRequest) error {
	token, err := adminToken(ctx)
	if err != nil {
		return err
	}

	user, err := kc.GetUserByID(ctx, token, keycloakRealm, keycloakID)
	if err != nil {
		return err
	}
	user.Email = gocloak.StringP(req.Email)
	user.FirstName = gocloak.StringP(req.FirstName)
	user.LastName = gocloak.StringP(req.LastName)
	return kc.UpdateUser(ctx, token, keycloakRealm, *user)
}

// disableKeycloakUser ปิดบัญชีใน Keycloak ทำให้ login เข้ามาใหม่ไม่ได้
func disableKeycloakUser(ctx context.Context, keycloakID string) error {
	token, err := adminToken(ctx)
	if err != nil {
		return err
	}

	user, err := kc.GetUserByID(ctx, token, keycloakRealm, keycloakID)
	if err != nil {
		return err
	}
	user.Enabled = gocloak.BoolP(false)
	return kc.UpdateUser(ctx, token, keycloakRealm, *user)
}

// setKeycloakRole ทำให้ user มี role ของระบบ (USER/STAFF/ADMIN) เหลืออยู่ตัวเดียวตามที่กำหนด
func setKeycloakRole(ctx context.Context, keycloakID, role string) error {
	token, err := adminToken(ctx)
	if err != nil {
		return err
	}

	current, err := kc.GetRealmRolesByUserID(ctx, token, keycloakRealm, keycloakID)
	if err != nil {
		return err
	}
	oldRoles := []gocloak.Role{}
	for _, r := range current {
		if validRoles[*r.Name] && *r.Name != role {
			oldRoles = append(oldRoles, *r)
		}
	}
	if len(oldRoles) > 0 {
		if err := kc.DeleteRealmRoleFromUser(ctx, token, keycloakRealm, keycloakID, oldRoles); err != nil {
			return err
		}
	}

	newRole, err := kc.GetRealmRole(ctx, token, keycloakRealm, role)
	if err != nil {
		return err
	}
	return kc.AddRealmRoleToUser(ctx, token, keycloakRealm, keycloakID, []gocloak.Role{*newRole})
}

// deleteKeycloakUser ใช้ลบบัญชีที่เพิ่งสร้าง เมื่อบันทึกลงฐานข้อมูลไม่สำเร็จ (rollback)
func deleteKeycloakUser(keycloakID string) {
	ctx := context.Background()
	token, err := adminToken(ctx)
	if err == nil {
		err = kc.DeleteUser(ctx, token, keycloakRealm, keycloakID)
	}
	if err != nil {
		log.Printf("rollback: cannot delete keycloak user %s: %v", keycloakID, err)
	}
}
