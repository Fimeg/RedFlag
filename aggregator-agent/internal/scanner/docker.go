package scanner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/aggregator-project/aggregator-agent/internal/client"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
)

// DockerScanner scans for Docker image updates
type DockerScanner struct {
	client         *dockerclient.Client
	registryClient *RegistryClient
}

// NewDockerScanner creates a new Docker scanner
func NewDockerScanner() (*DockerScanner, error) {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &DockerScanner{
		client:         cli,
		registryClient: NewRegistryClient(),
	}, nil
}

// IsAvailable checks if Docker is available on this system
func (s *DockerScanner) IsAvailable() bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}

	// Try to ping Docker daemon
	if s.client != nil {
		_, err := s.client.Ping(context.Background())
		return err == nil
	}

	return false
}

// Scan scans for available Docker image updates
func (s *DockerScanner) Scan() ([]client.UpdateReportItem, error) {
	ctx := context.Background()

	// List all containers
	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var updates []client.UpdateReportItem
	seenImages := make(map[string]bool)

	for _, c := range containers {
		imageName := c.Image

		// Skip if we've already checked this image
		if seenImages[imageName] {
			continue
		}
		seenImages[imageName] = true

		// Get current image details
		imageInspect, _, err := s.client.ImageInspectWithRaw(ctx, imageName)
		if err != nil {
			continue
		}

		// Parse image name and tag
		parts := strings.Split(imageName, ":")
		baseImage := parts[0]
		currentTag := "latest"
		if len(parts) > 1 {
			currentTag = parts[1]
		}

		// Check if update is available by comparing with registry
		hasUpdate, remoteDigest := s.checkForUpdate(ctx, baseImage, currentTag, imageInspect.ID)

		if hasUpdate {
			// Extract short digest for display (first 12 chars of sha256 hash)
			localDigest := imageInspect.ID
			remoteShortDigest := "unknown"
			if len(remoteDigest) > 7 {
				// Format: sha256:abcd... -> take first 12 chars of hash
				parts := strings.SplitN(remoteDigest, ":", 2)
				if len(parts) == 2 && len(parts[1]) >= 12 {
					remoteShortDigest = parts[1][:12]
				}
			}

			update := client.UpdateReportItem{
				PackageType:        "docker_image",
				PackageName:        imageName,
				PackageDescription: fmt.Sprintf("Container: %s", strings.Join(c.Names, ", ")),
				CurrentVersion:     localDigest[:12], // Short hash
				AvailableVersion:   remoteShortDigest,
				Severity:           "moderate",
				RepositorySource:   baseImage,
				Metadata: map[string]interface{}{
					"container_id":      c.ID[:12],
					"container_names":   c.Names,
					"container_state":   c.State,
					"image_created":     imageInspect.Created,
					"local_full_digest": localDigest,
					"remote_digest":     remoteDigest,
				},
			}

			updates = append(updates, update)
		}
	}

	return updates, nil
}

// checkForUpdate checks if a newer image version is available by comparing digests
// Returns (hasUpdate bool, remoteDigest string)
//
// This implementation:
// 1. Queries Docker Registry HTTP API v2 for remote manifest
// 2. Compares image digests (sha256 hashes) between local and remote
// 3. Handles authentication for Docker Hub (anonymous pull)
// 4. Caches registry responses (5 min TTL) to respect rate limits
// 5. Returns both the update status and remote digest for metadata
//
// Note: This compares exact digests. If local digest != remote digest, an update exists.
// This works for all tags including "latest", version tags, etc.
func (s *DockerScanner) checkForUpdate(ctx context.Context, imageName, tag, currentID string) (bool, string) {
	// Get remote digest from registry
	remoteDigest, err := s.registryClient.GetRemoteDigest(ctx, imageName, tag)
	if err != nil {
		// If we can't check the registry, log the error but don't report an update
		// This prevents false positives when registry is down or rate-limited
		fmt.Printf("Warning: Failed to check registry for %s:%s: %v\n", imageName, tag, err)
		return false, ""
	}

	// Compare digests
	// Local Docker image ID format: sha256:abc123...
	// Remote digest format: sha256:def456...
	// If they differ, an update is available
	hasUpdate := currentID != remoteDigest

	return hasUpdate, remoteDigest
}

// Close closes the Docker client
func (s *DockerScanner) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}
