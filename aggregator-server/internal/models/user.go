package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	Username     string     `json:"username" db:"username"`
	Email        string     `json:"email" db:"email"`
	PasswordHash string     `json:"-" db:"password_hash"` // Don't include in JSON
	Role         string     `json:"role" db:"role"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	LastLogin    *time.Time `json:"last_login" db:"last_login"`
}

type UserCredentials struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}