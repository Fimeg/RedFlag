package queries

import (
	"fmt"
	"log"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/doug-martin/goqu/v9"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// DockerQueries handles database operations for Docker images
type DockerQueries struct {
	db *sqlx.DB
}

func NewDockerQueries(db *sqlx.DB) *DockerQueries {
	return &DockerQueries{db: db}
}

// CreateDockerEventsBatch creates multiple Docker image events in a single transaction
func (q *DockerQueries) CreateDockerEventsBatch(events []models.StoredDockerImage) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := q.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare the insert statement
	stmt, err := tx.Prepare(`
		INSERT INTO docker_images (
			id, agent_id, package_type, package_name, current_version, available_version,
			severity, repository_source, metadata, event_type, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (agent_id, package_name, package_type, created_at) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert each event with error isolation
	for _, event := range events {
		_, err := stmt.Exec(
			event.ID,
			event.AgentID,
			event.PackageType,
			event.PackageName,
			event.CurrentVersion,
			event.AvailableVersion,
			event.Severity,
			event.RepositorySource,
			event.Metadata,
			event.EventType,
			event.CreatedAt,
		)
		if err != nil {
			// Log error but continue with other events
			log.Printf("[WARNING] [server] [database] docker_event_insert_failed event_id=%s error=%v", event.ID, err)
			continue
		}
	}

	return tx.Commit()
}

// GetDockerImages retrieves Docker images based on filter criteria
func (q *DockerQueries) GetDockerImages(filter *models.DockerFilter) (*models.DockerResult, error) {
	var images []models.StoredDockerImage

	sd := PG().From("docker_images")

	if filter.AgentID != nil {
		sd = sd.Where(goqu.Ex{"agent_id": *filter.AgentID})
	}
	if filter.ImageName != nil {
		sd = sd.Where(goqu.C("package_name").ILike("%" + *filter.ImageName + "%"))
	}
	if filter.Registry != nil {
		sd = sd.Where(goqu.C("repository_source").ILike("%" + *filter.Registry + "%"))
	}
	if filter.Severity != nil {
		sd = sd.Where(goqu.Ex{"severity": *filter.Severity})
	}
	if filter.HasUpdates != nil {
		if *filter.HasUpdates {
			sd = sd.Where(goqu.C("current_version").Neq(goqu.C("available_version")))
		} else {
			sd = sd.Where(goqu.C("current_version").Eq(goqu.C("available_version")))
		}
	}

	page := uint(1)
	pageSize := uint(50)
	if filter.Limit != nil {
		pageSize = uint(*filter.Limit)
		if filter.Offset != nil {
			page = uint(*filter.Offset / *filter.Limit) + 1
		}
	}

	cols := []string{"id", "agent_id", "package_type", "package_name", "current_version",
		"available_version", "severity", "repository_source", "metadata", "event_type", "created_at"}

	total, err := Paginated(q.db, sd, page, pageSize, goqu.C("created_at").Desc(), cols, &images)
	if err != nil {
		return nil, fmt.Errorf("failed to query docker images: %w", err)
	}

	return &models.DockerResult{
		Images:  images,
		Total:   total,
		Page:    int(page),
		PerPage: int(pageSize),
	}, nil
}

// GetDockerImagesByAgentID retrieves Docker images for a specific agent
func (q *DockerQueries) GetDockerImagesByAgentID(agentID uuid.UUID, limit int) ([]models.StoredDockerImage, error) {
	query := `
		SELECT id, agent_id, package_type, package_name, current_version, available_version,
		       severity, repository_source, metadata, event_type, created_at
		FROM docker_images
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := q.db.Query(query, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query docker images by agent: %w", err)
	}
	defer rows.Close()

	var images []models.StoredDockerImage
	for rows.Next() {
		var image models.StoredDockerImage
		err := rows.Scan(
			&image.ID,
			&image.AgentID,
			&image.PackageType,
			&image.PackageName,
			&image.CurrentVersion,
			&image.AvailableVersion,
			&image.Severity,
			&image.RepositorySource,
			&image.Metadata,
			&image.EventType,
			&image.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan docker image: %w", err)
		}
		images = append(images, image)
	}

	return images, nil
}

// GetDockerImagesWithUpdates retrieves Docker images that have available updates
func (q *DockerQueries) GetDockerImagesWithUpdates(limit int) ([]models.StoredDockerImage, error) {
	query := `
		SELECT id, agent_id, package_type, package_name, current_version, available_version,
		       severity, repository_source, metadata, event_type, created_at
		FROM docker_images
		WHERE current_version != available_version
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := q.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query docker images with updates: %w", err)
	}
	defer rows.Close()

	var images []models.StoredDockerImage
	for rows.Next() {
		var image models.StoredDockerImage
		err := rows.Scan(
			&image.ID,
			&image.AgentID,
			&image.PackageType,
			&image.PackageName,
			&image.CurrentVersion,
			&image.AvailableVersion,
			&image.Severity,
			&image.RepositorySource,
			&image.Metadata,
			&image.EventType,
			&image.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan docker image: %w", err)
		}
		images = append(images, image)
	}

	return images, nil
}

// DeleteOldDockerImages deletes Docker images older than the specified number of days
func (q *DockerQueries) DeleteOldDockerImages(days int) error {
	query := `DELETE FROM docker_images WHERE created_at < NOW() - INTERVAL '1 day' * $1`

	result, err := q.db.Exec(query, days)
	if err != nil {
		return fmt.Errorf("failed to delete old docker images: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected > 0 {
		log.Printf("[INFO] [server] [database] docker_cleanup removed=%d", rowsAffected)
	}

	return nil
}

// GetDockerStats returns statistics about Docker images across all agents
func (q *DockerQueries) GetDockerStats() (*models.DockerStats, error) {
	var stats models.DockerStats

	// Get total images
	err := q.db.QueryRow("SELECT COUNT(*) FROM docker_images").Scan(&stats.TotalImages)
	if err != nil {
		return nil, fmt.Errorf("failed to get total docker images: %w", err)
	}

	// Get images with updates
	err = q.db.QueryRow("SELECT COUNT(*) FROM docker_images WHERE current_version != available_version").Scan(&stats.UpdatesAvailable)
	if err != nil {
		return nil, fmt.Errorf("failed to get docker images with updates: %w", err)
	}

	// Get critical updates
	err = q.db.QueryRow("SELECT COUNT(*) FROM docker_images WHERE severity = 'critical' AND current_version != available_version").Scan(&stats.CriticalUpdates)
	if err != nil {
		return nil, fmt.Errorf("failed to get critical docker updates: %w", err)
	}

	// Get agents with Docker images
	err = q.db.QueryRow("SELECT COUNT(DISTINCT agent_id) FROM docker_images").Scan(&stats.AgentsWithContainers)
	if err != nil {
		return nil, fmt.Errorf("failed to get agents with docker images: %w", err)
	}

	return &stats, nil
}

// UpsertContainers replaces all containers for an agent with the latest report.
func (q *DockerQueries) UpsertContainers(agentID uuid.UUID, containers []models.AgentDockerContainer) error {
	if len(containers) == 0 {
		return nil
	}

	tx, err := q.db.Begin()
	if err != nil {
		return fmt.Errorf("docker_containers: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete old containers for this agent (full replace on each report)
	if _, err := tx.Exec(`DELETE FROM docker_containers WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("docker_containers: delete old: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO docker_containers
			(agent_id, container_id, name, image, image_id, state, health, stack_name, ports, created_at_epoch, labels, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (agent_id, container_id) DO UPDATE
			SET name = EXCLUDED.name,
			    image = EXCLUDED.image,
			    image_id = EXCLUDED.image_id,
			    state = EXCLUDED.state,
			    health = EXCLUDED.health,
			    stack_name = EXCLUDED.stack_name,
			    ports = EXCLUDED.ports,
			    created_at_epoch = EXCLUDED.created_at_epoch,
			    labels = EXCLUDED.labels,
			    last_seen_at = NOW()`)
	if err != nil {
		return fmt.Errorf("docker_containers: prepare: %w", err)
	}
	defer stmt.Close()

	for _, c := range containers {
		labelsJSON := models.JSONB{}
		for k, v := range c.Labels {
			labelsJSON[k] = v
		}
		if _, err := stmt.Exec(agentID, c.ContainerID, c.Name, c.Image, c.ImageID, c.State, c.Health, c.StackName, c.Ports, c.CreatedAt, labelsJSON); err != nil {
			log.Printf("[WARNING] [server] [database] docker_container_upsert_failed agent=%s container=%s error=%v", agentID, c.ContainerID, err)
		}
	}

	return tx.Commit()
}

// UpsertStacks replaces all stacks for an agent with the latest report.
func (q *DockerQueries) UpsertStacks(agentID uuid.UUID, stacks []models.AgentDockerStack) error {
	if len(stacks) == 0 {
		return nil
	}

	tx, err := q.db.Begin()
	if err != nil {
		return fmt.Errorf("docker_stacks: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM docker_stacks WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("docker_stacks: delete old: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO docker_stacks (agent_id, name, container_count, running_count, last_seen_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (agent_id, name) DO UPDATE
			SET container_count = EXCLUDED.container_count,
			    running_count = EXCLUDED.running_count,
			    last_seen_at = NOW()`)
	if err != nil {
		return fmt.Errorf("docker_stacks: prepare: %w", err)
	}
	defer stmt.Close()

	for _, s := range stacks {
		if _, err := stmt.Exec(agentID, s.Name, s.ContainerCount, s.RunningCount); err != nil {
			log.Printf("[WARNING] [server] [database] docker_stack_upsert_failed agent=%s stack=%s error=%v", agentID, s.Name, err)
		}
	}

	return tx.Commit()
}

// UpdateDockerEngineVersion stores the Docker engine version on the agent row.
func (q *DockerQueries) UpdateDockerEngineVersion(agentID uuid.UUID, version string) error {
	if version == "" {
		return nil
	}
	_, err := q.db.Exec(`UPDATE agents SET docker_version = $1 WHERE id = $2`, version, agentID)
	if err != nil {
		return fmt.Errorf("docker_version: update: %w", err)
	}
	return nil
}

// GetDockerContainers returns all containers for an agent.
func (q *DockerQueries) GetDockerContainers(agentID uuid.UUID) ([]models.StoredDockerContainer, error) {
	var rows []models.StoredDockerContainer
	err := q.db.Select(&rows, `SELECT * FROM docker_containers WHERE agent_id = $1 ORDER BY name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("docker_containers: get: %w", err)
	}
	return rows, nil
}

// GetDockerStacks returns all stacks for an agent.
func (q *DockerQueries) GetDockerStacks(agentID uuid.UUID) ([]models.StoredDockerStack, error) {
	var rows []models.StoredDockerStack
	err := q.db.Select(&rows, `SELECT * FROM docker_stacks WHERE agent_id = $1 ORDER BY name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("docker_stacks: get: %w", err)
	}
	return rows, nil
}

// GetDockerContainersFleet returns containers across all agents.
func (q *DockerQueries) GetDockerContainersFleet() ([]models.StoredDockerContainer, error) {
	var rows []models.StoredDockerContainer
	err := q.db.Select(&rows, `SELECT * FROM docker_containers ORDER BY agent_id, name`)
	if err != nil {
		return nil, fmt.Errorf("docker_containers: fleet: %w", err)
	}
	return rows, nil
}

// GetDockerStacksFleet returns stacks across all agents.
func (q *DockerQueries) GetDockerStacksFleet() ([]models.StoredDockerStack, error) {
	var rows []models.StoredDockerStack
	err := q.db.Select(&rows, `SELECT * FROM docker_stacks ORDER BY agent_id, name`)
	if err != nil {
		return nil, fmt.Errorf("docker_stacks: fleet: %w", err)
	}
	return rows, nil
}