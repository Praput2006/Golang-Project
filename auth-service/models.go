package main

import (
	"time"
)

const (
	RoleUser  = "USER"
	RoleStaff = "STAFF"
	RoleAdmin = "ADMIN"

	StatusActive      = "ACTIVE"
	StatusDeactivated = "DEACTIVATED"
)

var validRoles = map[string]bool{RoleUser: true, RoleStaff: true, RoleAdmin: true}
var validStatuses = map[string]bool{StatusActive: true, StatusDeactivated: true}

// User ตรงกับตาราง users ที่สร้างใน migrations/001_create_users_table.sql
// GORM แปลงชื่อ field เป็นชื่อคอลัมน์ให้เอง เช่น KeycloakID → keycloak_id
// และเติม CreatedAt / UpdatedAt ให้อัตโนมัติตอนสร้างและแก้ไข
type User struct {
	ID         int64     `json:"id"`
	KeycloakID string    `json:"keycloak_id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	Role       string    `json:"role"`   // USER, STAFF, ADMIN
	Status     string    `json:"status"` // ACTIVE, DEACTIVATED
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RegisterRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=50"`
	Email     string `json:"email" binding:"required,email,max=100"`
	Password  string `json:"password" binding:"required,min=8,max=72"`
	FirstName string `json:"first_name" binding:"required,max=50"`
	LastName  string `json:"last_name" binding:"required,max=50"`
}

type UpdateUserRequest struct {
	Email     string `json:"email" binding:"required,email,max=100"`
	FirstName string `json:"first_name" binding:"required,max=50"`
	LastName  string `json:"last_name" binding:"required,max=50"`
}

type ChangeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=USER STAFF ADMIN"`
}

type UserSummary struct {
	Total         int64            `json:"total"`
	ByRole        map[string]int64 `json:"by_role"`
	ByStatus      map[string]int64 `json:"by_status"`
	NewLast30Days int64            `json:"new_last_30_days"`
}
