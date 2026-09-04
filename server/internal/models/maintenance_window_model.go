package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// MaintenanceWindow represents a recurring weekly maintenance window
// for gating update installations. DayOfWeek uses Go convention (0=Sunday).
type MaintenanceWindow struct {
	ID          uuid.UUID `json:"id" db:"id"`
	DayOfWeek   int       `json:"day_of_week" db:"day_of_week"`   // 0=Sunday, 6=Saturday
	StartTime   string    `json:"start_time" db:"start_time"`     // HH:MM (TIME column)
	EndTime     string    `json:"end_time" db:"end_time"`         // HH:MM (TIME column)
	Description string    `json:"description" db:"description"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// MaintenanceWindowInput is the request body for create/update operations.
type MaintenanceWindowInput struct {
	DayOfWeek   int    `json:"day_of_week"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}
