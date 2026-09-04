package queries

import (
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// PackageQueries provides an alias for AgentUpdateQueries to match the expected interface
// This maintains backward compatibility while using the existing agent update package system
type PackageQueries struct {
	*AgentUpdateQueries
}

// NewPackageQueries creates a new PackageQueries instance
func NewPackageQueries(db *sqlx.DB) *PackageQueries {
	return &PackageQueries{
		AgentUpdateQueries: NewAgentUpdateQueries(db),
	}
}

// StoreSignedPackage stores a signed agent package (alias for CreateUpdatePackage)
func (pq *PackageQueries) StoreSignedPackage(pkg *models.AgentUpdatePackage) error {
	return pq.CreateUpdatePackage(pkg)
}

// GetSignedPackage retrieves a signed package (alias for GetUpdatePackageByVersion)
func (pq *PackageQueries) GetSignedPackage(version, platform, architecture string) (*models.AgentUpdatePackage, error) {
	return pq.GetUpdatePackageByVersion(version, platform, architecture)
}

// GetSignedPackageByID retrieves a signed package by ID (alias for GetUpdatePackage)
func (pq *PackageQueries) GetSignedPackageByID(id uuid.UUID) (*models.AgentUpdatePackage, error) {
	return pq.GetUpdatePackage(id)
}