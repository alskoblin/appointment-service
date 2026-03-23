package domain

import (
	"fmt"
	"strings"
	"time"
)

type UserRole string

const (
	RoleAdmin      UserRole = "admin"
	RoleClient     UserRole = "client"
	RoleSpecialist UserRole = "specialist"
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         UserRole
	ClientID     *int64
	SpecialistID *int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Identity struct {
	UserID       int64
	Email        string
	Role         UserRole
	ClientID     *int64
	SpecialistID *int64
}

func ParseUserRole(raw string) (UserRole, error) {
	switch UserRole(strings.ToLower(strings.TrimSpace(raw))) {
	case RoleAdmin:
		return RoleAdmin, nil
	case RoleClient:
		return RoleClient, nil
	case RoleSpecialist:
		return RoleSpecialist, nil
	default:
		return "", fmt.Errorf("role must be one of: admin, client, specialist")
	}
}
