package queries

import (
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type MaintenanceWindowQueries struct {
	db *sqlx.DB
}

func NewMaintenanceWindowQueries(db *sqlx.DB) *MaintenanceWindowQueries {
	return &MaintenanceWindowQueries{db: db}
}

func (q *MaintenanceWindowQueries) List() ([]models.MaintenanceWindow, error) {
	var windows []models.MaintenanceWindow
	query := `SELECT * FROM maintenance_windows ORDER BY day_of_week, start_time`
	err := q.db.Select(&windows, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list maintenance windows: %w", err)
	}
	return windows, nil
}

func (q *MaintenanceWindowQueries) GetByID(id uuid.UUID) (*models.MaintenanceWindow, error) {
	var window models.MaintenanceWindow
	query := `SELECT * FROM maintenance_windows WHERE id = $1`
	err := q.db.Get(&window, query, id)
	if err != nil {
		return nil, fmt.Errorf("maintenance window not found: %w", err)
	}
	return &window, nil
}

func (q *MaintenanceWindowQueries) Create(input models.MaintenanceWindowInput) (*models.MaintenanceWindow, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	var window models.MaintenanceWindow
	query := `
		INSERT INTO maintenance_windows (day_of_week, start_time, end_time, description, enabled)
		VALUES ($1, $2::time, $3::time, $4, $5)
		RETURNING *`
	err := q.db.Get(&window, query, input.DayOfWeek, input.StartTime, input.EndTime, input.Description, enabled)
	if err != nil {
		return nil, fmt.Errorf("failed to create maintenance window: %w", err)
	}
	return &window, nil
}

func (q *MaintenanceWindowQueries) Update(id uuid.UUID, input models.MaintenanceWindowInput) error {
	query := `
		UPDATE maintenance_windows
		SET day_of_week = $1, start_time = $2::time, end_time = $3::time,
		    description = $4, updated_at = NOW()`
	args := []interface{}{input.DayOfWeek, input.StartTime, input.EndTime, input.Description}

	if input.Enabled != nil {
		query += `, enabled = $5`
		args = append(args, *input.Enabled)
	}
	query += ` WHERE id = $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, id)

	result, err := q.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update maintenance window: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("maintenance window not found")
	}
	return nil
}

func (q *MaintenanceWindowQueries) Delete(id uuid.UUID) error {
	result, err := q.db.Exec(`DELETE FROM maintenance_windows WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete maintenance window: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("maintenance window not found")
	}
	return nil
}

func (q *MaintenanceWindowQueries) IsWithinMaintenanceWindow(now time.Time) (bool, error) {
	pgDow := int(now.Weekday())
	currentTime := now.Format("15:04")

	query := `
		SELECT
		  NOT EXISTS (SELECT 1 FROM maintenance_windows WHERE enabled = true)
		  OR EXISTS (
		    SELECT 1 FROM maintenance_windows
		    WHERE enabled = true
		      AND day_of_week = $1
		      AND (
		        (start_time < end_time AND start_time <= $2::time AND end_time > $2::time)
		        OR
		        (start_time > end_time AND (start_time <= $2::time OR end_time > $2::time))
		      )
		  )`

	var inside bool
	err := q.db.Get(&inside, query, pgDow, currentTime)
	if err != nil {
		return false, fmt.Errorf("failed to check maintenance window: %w", err)
	}
	return inside, nil
}
