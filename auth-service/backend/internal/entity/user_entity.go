package entity

import "time"

// Domain Entity
type User struct {
	ID           uint64
	Username     string
	PasswordHash []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
