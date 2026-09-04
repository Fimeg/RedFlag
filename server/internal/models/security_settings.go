package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// SecuritySetting represents a user-configurable security setting
type SecuritySetting struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Category    string     `json:"category" db:"category"`
	Key         string     `json:"key" db:"key"`
	Value       string     `json:"value" db:"value"`
	IsEncrypted bool       `json:"is_encrypted" db:"is_encrypted"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at" db:"updated_at"`
	CreatedBy   *uuid.UUID `json:"created_by" db:"created_by"`
	UpdatedBy   *uuid.UUID `json:"updated_by" db:"updated_by"`
}

// SecuritySettingAudit represents an audit log entry for security setting changes
type SecuritySettingAudit struct {
	ID        uuid.UUID `json:"id" db:"id"`
	SettingID uuid.UUID `json:"setting_id" db:"setting_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Action    string    `json:"action" db:"action"` // create, update, delete
	OldValue  *string   `json:"old_value" db:"old_value"`
	NewValue  *string   `json:"new_value" db:"new_value"`
	Reason    string    `json:"reason" db:"reason"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}