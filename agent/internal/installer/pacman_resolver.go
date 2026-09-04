package installer

// PacmanResolvedArtifact is one exact archive in the transaction pacman
// resolved for a requested package. Paths remain valid until Cleanup is called.
type PacmanResolvedArtifact struct {
	Name            string
	Version         string
	Repository      string
	ArchivePath     string
	ArchiveSHA256   string
	SignaturePath   string
	SignatureSHA256 string
}

// PacmanResolution owns the private sync database and cache backing a resolved
// transaction. The caller keeps it alive through helper execution, then calls
// Cleanup even when mint or execution refuses.
type PacmanResolution struct {
	Artifacts []PacmanResolvedArtifact
	Cleanup   func()
}
