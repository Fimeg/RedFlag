package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// SigningKey represents a signing key record in the database
type SigningKey struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	KeyID        string     `json:"key_id" db:"key_id"`
	PublicKey    string     `json:"public_key" db:"public_key"`
	Algorithm    string     `json:"algorithm" db:"algorithm"`
	IsActive     bool       `json:"is_active" db:"is_active"`
	IsPrimary    bool       `json:"is_primary" db:"is_primary"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	DeprecatedAt *time.Time `json:"deprecated_at,omitempty" db:"deprecated_at"`
	Version      int        `json:"version" db:"version"`
}
