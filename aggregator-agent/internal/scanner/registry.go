package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

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
	// Parse image name to determine registry and repository
	registry, repository := parseImageName(imageName)

	// Check cache first
	cacheKey := fmt.Sprintf("%s/%s:%s", registry, repository, tag)
	if digest := c.cache.get(cacheKey); digest != "" {
		return digest, nil
	}

	// Get authentication token (if needed)
	token, err := c.getAuthToken(ctx, registry, repository)
	if err != nil {
		return "", fmt.Errorf("failed to get auth token: %w", err)
	}

	// Fetch manifest from registry
	digest, err := c.fetchManifestDigest(ctx, registry, repository, tag, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Cache the result (5 minute TTL to avoid hammering registries)
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

	// Check if first part looks like a domain (contains . or :)
	if len(parts) >= 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		// Custom registry: gcr.io/myproject/myimage
		registry = parts[0]
		repository = strings.Join(parts[1:], "/")
	} else if len(parts) == 1 {
		// Official image: nginx -> library/nginx
		registry = "registry-1.docker.io"
		repository = "library/" + parts[0]
	} else {
		// User image: myuser/myimage
		registry = "registry-1.docker.io"
		repository = imageName
	}

	return registry, repository
}

// getAuthToken obtains an authentication token for the registry
// For Docker Hub, uses the token authentication flow
// For other registries, may need different auth mechanisms (TODO: implement)
func (c *RegistryClient) getAuthToken(ctx context.Context, registry, repository string) (string, error) {
	// Docker Hub token authentication
	if registry == "registry-1.docker.io" {
		return c.getDockerHubToken(ctx, repository)
	}

	// For other registries, we'll try unauthenticated first
	// TODO: Support authentication for private registries (basic auth, bearer tokens, etc.)
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

	// Docker Hub can return either 'token' or 'access_token'
	if tokenResp.Token != "" {
		return tokenResp.Token, nil
	}
	return tokenResp.AccessToken, nil
}

// fetchManifestDigest fetches the manifest from the registry and extracts the digest
func (c *RegistryClient) fetchManifestDigest(ctx context.Context, registry, repository, tag, token string) (string, error) {
	// Build manifest URL
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return "", err
	}

	// Set required headers
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

	// Try to get digest from Docker-Content-Digest header first (faster)
	if digest := resp.Header.Get("Docker-Content-Digest"); digest != "" {
		return digest, nil
	}

	// Fallback: parse manifest and extract config digest
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
		// Entry expired
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

// cleanupExpired removes expired entries from the cache (called periodically)
func (mc *manifestCache) cleanupExpired() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	for key, entry := range mc.entries {
		if now.After(entry.expiresAt) {
			delete(mc.entries, key)
		}
	}
}
