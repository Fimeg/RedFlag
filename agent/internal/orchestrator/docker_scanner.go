package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
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

	if s.client != nil {
		_, err := s.client.Ping(context.Background())
		return err == nil
	}

	return false
}

// Scan collects Docker image info and returns UpdateReportItems with full data in Metadata
func (s *DockerScanner) Scan() ([]client.UpdateReportItem, error) {
	ctx := context.Background()

	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var items []client.UpdateReportItem
	seenImages := make(map[string]bool)

	for _, c := range containers {
		imageName := c.Image
		if seenImages[imageName] {
			continue
		}
		seenImages[imageName] = true

		imageInspect, _, err := s.client.ImageInspectWithRaw(ctx, imageName)
		if err != nil {
			continue
		}

		parts := strings.Split(imageName, ":")
		baseImage := parts[0]
		currentTag := "latest"
		if len(parts) > 1 {
			currentTag = parts[1]
		}

		hasUpdate, remoteDigest := s.checkForUpdate(ctx, baseImage, currentTag, imageInspect.ID)

		localDigest := imageInspect.ID
		localShortDigest := ""
		if len(localDigest) > 7 {
			parts := strings.SplitN(localDigest, ":", 2)
			if len(parts) == 2 && len(parts[1]) >= 12 {
				localShortDigest = parts[1][:12]
			}
		}

		remoteShortDigest := ""
		if len(remoteDigest) > 7 {
			parts := strings.SplitN(remoteDigest, ":", 2)
			if len(parts) == 2 && len(parts[1]) >= 12 {
				remoteShortDigest = parts[1][:12]
			}
		}

		severity := "low"
		if hasUpdate {
			severity = "moderate"
		}

		labels := make(map[string]string)
		if imageInspect.Config != nil {
			labels = imageInspect.Config.Labels
		}

		sizeBytes := int64(0)
		if len(imageInspect.RootFS.Layers) > 0 {
			sizeBytes = imageInspect.Size
		}

		createdAt := imageInspect.Created
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}

		items = append(items, client.UpdateReportItem{
			PackageType:        "docker_image",
			PackageName:        imageName,
			PackageDescription: fmt.Sprintf("Docker image %s:%s", imageName, currentTag),
			CurrentVersion:     localShortDigest,
			AvailableVersion:   remoteShortDigest,
			Severity:           severity,
			RepositorySource:   baseImage,
			SizeBytes:          sizeBytes,
			Metadata: map[string]interface{}{
				"image_name":       imageName,
				"image_tag":        currentTag,
				"image_id":         localShortDigest,
				"latest_image_id":  remoteShortDigest,
				"has_update":       hasUpdate,
				"repository":       baseImage,
				"size_bytes":       sizeBytes,
				"created_at":       createdAt,
				"labels":           labels,
				"container_id":     c.ID[:12],
				"container_names":  c.Names,
				"container_state":  c.State,
				"image_created":    imageInspect.Created,
				"local_full_digest": localDigest,
				"remote_digest":    remoteDigest,
			},
		})
	}

	return items, nil
}

// Name returns the scanner name
func (s *DockerScanner) Name() string {
	return "Docker Image Scanner"
}

// ScanContainers returns container-level data for enriched Docker reports.
func (s *DockerScanner) ScanContainers() ([]client.DockerReportContainer, error) {
	ctx := context.Background()

	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []client.DockerReportContainer
	for _, c := range containers {
		stackName := ""
		if v, ok := c.Labels["com.docker.stack.namespace"]; ok {
			stackName = v
		}
		if v, ok := c.Labels["com.docker.compose.project"]; ok && stackName == "" {
			stackName = v
		}

		health := ""
		if c.State == "running" {
			inspect, err := s.client.ContainerInspect(ctx, c.ID)
			if err == nil && inspect.State != nil && inspect.State.Health != nil {
				health = inspect.State.Health.Status
			}
		}

		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		ports := ""
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				ports += fmt.Sprintf("%s:%d->%d/%s ", p.IP, p.PublicPort, p.PrivatePort, p.Type)
			}
		}

		result = append(result, client.DockerReportContainer{
			ContainerID: c.ID[:12],
			Name:        name,
			Image:       c.Image,
			ImageID:     c.ImageID,
			State:       c.State,
			Health:      health,
			StackName:   stackName,
			Ports:       strings.TrimSpace(ports),
			CreatedAt:   c.Created,
			Labels:      c.Labels,
		})
	}

	return result, nil
}

// ScanStacks aggregates containers into compose stacks.
func (s *DockerScanner) ScanStacks(containers []client.DockerReportContainer) []client.DockerReportStack {
	stackMap := make(map[string]*client.DockerReportStack)
	for _, c := range containers {
		if c.StackName == "" {
			continue
		}
		s, ok := stackMap[c.StackName]
		if !ok {
			s = &client.DockerReportStack{Name: c.StackName}
			stackMap[c.StackName] = s
		}
		s.ContainerCount++
		if c.State == "running" {
			s.RunningCount++
		}
	}

	result := make([]client.DockerReportStack, 0, len(stackMap))
	for _, s := range stackMap {
		result = append(result, *s)
	}
	return result
}

// GetEngineVersion returns the Docker engine version string.
func (s *DockerScanner) GetEngineVersion() string {
	ctx := context.Background()
	v, err := s.client.ServerVersion(ctx)
	if err != nil {
		return ""
	}
	return v.Version
}

// Close closes the Docker client
func (s *DockerScanner) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// checkForUpdate checks if a newer image version is available by comparing digests
func (s *DockerScanner) checkForUpdate(ctx context.Context, imageName, tag, currentID string) (bool, string) {
	remoteDigest, err := s.registryClient.GetRemoteDigest(ctx, imageName, tag)
	if err != nil {
		log.Printf("[WARNING] [agent] [docker] registry_check_failed image=%s:%s error=%v", imageName, tag, err)
		return false, ""
	}

	return currentID != remoteDigest, remoteDigest
}

// --- Registry Client ---

// RegistryClient handles communication with Docker registries (Docker Hub and custom registries)
type RegistryClient struct {
	httpClient *http.Client
	cache      *manifestCache
}

// manifestCache stores registry responses to avoid hitting rate limits
type manifestCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	digest    string
	expiresAt time.Time
}

// ManifestResponse represents the response from a Docker Registry API v2 manifest request
type ManifestResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

// DockerHubTokenResponse represents the authentication token response from Docker Hub
type DockerHubTokenResponse struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

// NewRegistryClient creates a new registry client with caching
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: &manifestCache{
			entries: make(map[string]*cacheEntry),
		},
	}
}

// GetRemoteDigest fetches the digest of a remote image from the registry
// Returns the digest string (e.g., "sha256:abc123...") or an error
func (c *RegistryClient) GetRemoteDigest(ctx context.Context, imageName, tag string) (string, error) {
	registry, repository := parseImageName(imageName)

	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	if digest := c.cache.get(cacheKey); digest != "" {
		return digest, nil
	}

	token, err := c.getAuthToken(ctx, registry, repository)
	if err != nil {
		return "", fmt.Errorf("failed to get auth token: %w", err)
	}

	digest, err := c.fetchManifestDigest(ctx, registry, repository, tag, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}

	c.cache.set(cacheKey, digest, 5*time.Minute)
	return digest, nil
}

// parseImageName splits an image name into registry and repository
// Examples:
//   - "nginx" -> ("registry-1.docker.io", "library/nginx")
//   - "myuser/myimage" -> ("registry-1.docker.io", "myuser/myimage")
//   - "gcr.io/myproject/myimage" -> ("gcr.io", "myproject/myimage")
func parseImageName(imageName string) (registry, repository string) {
	parts := strings.Split(imageName, "/")

	if len(parts) >= 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repository = strings.Join(parts[1:], "/")
	} else if len(parts) == 1 {
		registry = "registry-1.docker.io"
		repository = "library/" + parts[0]
	} else {
		registry = "registry-1.docker.io"
		repository = imageName
	}

	return registry, repository
}

// getAuthToken obtains an authentication token for the registry
func (c *RegistryClient) getAuthToken(ctx context.Context, registry, repository string) (string, error) {
	if registry == "registry-1.docker.io" {
		return c.getDockerHubToken(ctx, repository)
	}

	return "", nil
}

// getDockerHubToken obtains a token from Docker Hub's authentication service
func (c *RegistryClient) getDockerHubToken(ctx context.Context, repository string) (string, error) {
	authURL := fmt.Sprintf(
		"https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull",
		repository,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp DockerHubTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.Token != "" {
		return tokenResp.Token, nil
	}
	return tokenResp.AccessToken, nil
}

// fetchManifestDigest fetches the manifest from the registry and extracts the digest
func (c *RegistryClient) fetchManifestDigest(ctx context.Context, registry, repository, tag, token string) (string, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("rate limited by registry (429 Too Many Requests)")
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("unauthorized: authentication failed for %s/%s:%s", registry, repository, tag)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("manifest request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if digest := resp.Header.Get("Docker-Content-Digest"); digest != "" {
		return digest, nil
	}

	var manifest ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", fmt.Errorf("failed to decode manifest: %w", err)
	}

	if manifest.Config.Digest == "" {
		return "", fmt.Errorf("manifest does not contain a config digest")
	}

	return manifest.Config.Digest, nil
}

// manifestCache methods

func (mc *manifestCache) get(key string) string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	entry, exists := mc.entries[key]
	if !exists {
		return ""
	}

	if time.Now().After(entry.expiresAt) {
		delete(mc.entries, key)
		return ""
	}

	return entry.digest
}

func (mc *manifestCache) set(key, digest string, ttl time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.entries[key] = &cacheEntry{
		digest:    digest,
		expiresAt: time.Now().Add(ttl),
	}
}

// ScanInventory returns Docker images as InventoryItems. Implements the
// InventoryScanner interface. This is the inventory path — distinct from
// Scan() which returns UpdateReportItems for the update path.
func (s *DockerScanner) ScanInventory() ([]client.InventoryItem, error) {
	ctx := context.Background()

	containers, err := s.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var items []client.InventoryItem
	seenImages := make(map[string]bool)

	for _, c := range containers {
		imageName := c.Image
		if seenImages[imageName] {
			continue
		}
		seenImages[imageName] = true

		imageInspect, _, err := s.client.ImageInspectWithRaw(ctx, imageName)
		if err != nil {
			continue
		}

		parts := strings.Split(imageName, ":")
		baseImage := parts[0]
		currentTag := "latest"
		if len(parts) > 1 {
			currentTag = parts[1]
		}

		localDigest := imageInspect.ID
		localShortDigest := ""
		if len(localDigest) > 7 {
			digestParts := strings.SplitN(localDigest, ":", 2)
			if len(digestParts) == 2 && len(digestParts[1]) >= 12 {
				localShortDigest = digestParts[1][:12]
			}
		}

		hasUpdate, remoteDigest := s.checkForUpdate(ctx, baseImage, currentTag, imageInspect.ID)

		remoteShortDigest := ""
		if len(remoteDigest) > 7 {
			digestParts := strings.SplitN(remoteDigest, ":", 2)
			if len(digestParts) == 2 && len(digestParts[1]) >= 12 {
				remoteShortDigest = digestParts[1][:12]
			}
		}

		labels := make(map[string]string)
		if imageInspect.Config != nil {
			labels = imageInspect.Config.Labels
		}

		sizeBytes := imageInspect.Size

		createdAt := imageInspect.Created
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}

		items = append(items, client.InventoryItem{
			Ecosystem:   "docker",
			ItemName:    imageName,
			ItemVersion: localShortDigest,
			Description: fmt.Sprintf("Docker image %s:%s", baseImage, currentTag),
			SizeBytes:   sizeBytes,
			Vendor:      baseImage,
			Metadata: map[string]interface{}{
				"image_tag":        currentTag,
				"image_id":         localShortDigest,
				"latest_image_id":  remoteShortDigest,
				"has_update":       hasUpdate,
				"repository":       baseImage,
				"created_at":       createdAt,
				"labels":           labels,
				"container_id":     c.ID[:12],
				"container_names":  c.Names,
				"container_state":  c.State,
				"local_full_digest": localDigest,
				"remote_digest":    remoteDigest,
			},
		})
	}

	return items, nil
}
