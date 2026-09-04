package queries

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type SecuritySettingsQueries struct {
	db *sqlx.DB
}

func NewSecuritySettingsQueries(db *sqlx.DB) *SecuritySettingsQueries {
	return &SecuritySettingsQueries{db: db}
}

// GetSetting retrieves a specific security setting by category and key
func (q *SecuritySettingsQueries) GetSetting(category, key string) (*models.SecuritySetting, error) {
	query := `
		SELECT id, category, key, value, is_encrypted, created_at, updated_at, created_by, updated_by
		FROM security_settings
		WHERE category = $1 AND key = $2
	`

	var setting models.SecuritySetting
	err := q.db.Get(&setting, query, category, key)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get security setting: %w", err)
	}

	return &setting, nil
}

// GetAllSettings retrieves all security settings
func (q *SecuritySettingsQueries) GetAllSettings() ([]models.SecuritySetting, error) {
	query := `
		SELECT id, category, key, value, is_encrypted, created_at, updated_at, created_by, updated_by
		FROM security_settings
		ORDER BY category, key
	`

	var settings []models.SecuritySetting
	err := q.db.Select(&settings, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all security settings: %w", err)
	}

	return settings, nil
}

// GetSettingsByCategory retrieves all settings for a specific category
func (q *SecuritySettingsQueries) GetSettingsByCategory(category string) ([]models.SecuritySetting, error) {
	query := `
		SELECT id, category, key, value, is_encrypted, created_at, updated_at, created_by, updated_by
		FROM security_settings
		WHERE category = $1
		ORDER BY key
	`

	var settings []models.SecuritySetting
	err := q.db.Select(&settings, query, category)
	if err != nil {
		return nil, fmt.Errorf("failed to get security settings by category: %w", err)
	}

	return settings, nil
}

// CreateSetting persists a setting. value is the exact string to store in the
// value column; the caller (SecuritySettingsService) owns JSON serialization and,
// for sensitive keys, encryption — the encryption key lives on the service, not
// here. is_encrypted records whether value is ciphertext (SEC-024).
func (q *SecuritySettingsQueries) CreateSetting(category, key, value string, isEncrypted bool, createdBy *uuid.UUID) (*models.SecuritySetting, error) {
	setting := &models.SecuritySetting{
		ID:          uuid.Must(uuid.NewV4()),
		Category:    category,
		Key:         key,
		Value:       value,
		IsEncrypted: isEncrypted,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   createdBy,
	}

	query := `
		INSERT INTO security_settings (
			id, category, key, value, is_encrypted, created_at, created_by
		) VALUES (
			:id, :category, :key, :value, :is_encrypted, :created_at, :created_by
		)
		RETURNING *
	`

	rows, err := q.db.NamedQuery(query, setting)
	if err != nil {
		return nil, fmt.Errorf("failed to create security setting: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var createdSetting models.SecuritySetting
		if err := rows.StructScan(&createdSetting); err != nil {
			return nil, fmt.Errorf("failed to scan created setting: %w", err)
		}
		return &createdSetting, nil
	}

	return nil, fmt.Errorf("failed to create security setting: no rows returned")
}

// UpdateSetting updates an existing security setting
func (q *SecuritySettingsQueries) UpdateSetting(category, key, value string, isEncrypted bool, updatedBy *uuid.UUID) (*models.SecuritySetting, *string, error) {
	// Get the old value first
	oldSetting, err := q.GetSetting(category, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get old setting: %w", err)
	}
	if oldSetting == nil {
		return nil, nil, fmt.Errorf("setting not found")
	}

	var oldValue *string
	if oldSetting != nil {
		oldValue = &oldSetting.Value
	}

	// value is the exact column content the service produced (plain JSON, or
	// ciphertext for sensitive keys); persist it and the is_encrypted flag as-is.
	now := time.Now().UTC()
	query := `
		UPDATE security_settings
		SET value = $1, is_encrypted = $2, updated_at = $3, updated_by = $4
		WHERE category = $5 AND key = $6
		RETURNING id, category, key, value, is_encrypted, created_at, updated_at, created_by, updated_by
	`

	var updatedSetting models.SecuritySetting
	err = q.db.QueryRow(query, value, isEncrypted, now, updatedBy, category, key).Scan(
		&updatedSetting.ID,
		&updatedSetting.Category,
		&updatedSetting.Key,
		&updatedSetting.Value,
		&updatedSetting.IsEncrypted,
		&updatedSetting.CreatedAt,
		&updatedSetting.UpdatedAt,
		&updatedSetting.CreatedBy,
		&updatedSetting.UpdatedBy,
	)

	if err != nil {
		return nil, oldValue, fmt.Errorf("failed to update security setting: %w", err)
	}

	return &updatedSetting, oldValue, nil
}

// DeleteSetting deletes a security setting
func (q *SecuritySettingsQueries) DeleteSetting(category, key string) (*string, error) {
	// Get the old value first
	oldSetting, err := q.GetSetting(category, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get old setting: %w", err)
	}
	if oldSetting == nil {
		return nil, nil
	}

	query := `
		DELETE FROM security_settings
		WHERE category = $1 AND key = $2
		RETURNING value
	`

	var oldValue string
	err = q.db.QueryRow(query, category, key).Scan(&oldValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to delete security setting: %w", err)
	}

	return &oldValue, nil
}

// CreateAuditLog creates an audit log entry for setting changes
func (q *SecuritySettingsQueries) CreateAuditLog(settingID, userID uuid.UUID, action, oldValue, newValue, reason string) error {
	audit := &models.SecuritySettingAudit{
		ID:        uuid.Must(uuid.NewV4()),
		SettingID: settingID,
		UserID:    userID,
		Action:    action,
		OldValue:  &oldValue,
		NewValue:  &newValue,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	}

	// Handle null values for old/new values
	if oldValue == "" {
		audit.OldValue = nil
	}
	if newValue == "" {
		audit.NewValue = nil
	}

	query := `
		INSERT INTO security_settings_audit (
			id, setting_id, changed_by, previous_value, new_value, reason, changed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
	`

	_, err := q.db.Exec(query, audit.ID, audit.SettingID, audit.UserID, audit.OldValue, audit.NewValue, audit.Reason, audit.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

// GetAuditLogs retrieves audit logs for a setting
// GetAllAuditLogs returns recent audit trail entries across all settings (F-E1-7 fix)
func (q *SecuritySettingsQueries) GetAllAuditLogs(limit int) ([]models.SecuritySettingAudit, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT sa.id, sa.setting_id, sa.previous_value as old_value, sa.new_value,
		       sa.reason, sa.changed_at as created_at, sa.changed_by as user_id
		FROM security_settings_audit sa
		ORDER BY sa.changed_at DESC
		LIMIT $1
	`
	var audits []models.SecuritySettingAudit
	err := q.db.Select(&audits, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get all audit logs: %w", err)
	}
	return audits, nil
}

func (q *SecuritySettingsQueries) GetAuditLogs(category, key string, limit int) ([]models.SecuritySettingAudit, error) {
	query := `
		SELECT sa.id, sa.setting_id, sa.changed_by as user_id, sa.previous_value as old_value,
		       sa.new_value, sa.reason, sa.changed_at as created_at
		FROM security_settings_audit sa
		INNER JOIN security_settings s ON sa.setting_id = s.id
		WHERE s.category = $1 AND s.key = $2
		ORDER BY sa.changed_at DESC
		LIMIT $3
	`

	var audits []models.SecuritySettingAudit
	err := q.db.Select(&audits, query, category, key, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs: %w", err)
	}

	return audits, nil
}