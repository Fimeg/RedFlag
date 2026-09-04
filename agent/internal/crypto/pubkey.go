package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/event"
)

var teeLogger *event.TeeLogger

// InitLogger sets the package-level TeeLogger for dual-output logging.
func InitLogger(l *event.TeeLogger) {
	teeLogger = l
}

const defaultCacheTTLHours = 24

// Stale-cache fallback window (SEC-028). When the server is unreachable the
// agent keeps working on its last-known public key, but only for a bounded
// time: a key the server rotated OUT must not stay trusted forever just because
// the agent can't phone home. The WINDOW LENGTH is operator policy delivered via
// security settings (command_signing.stale_key_max_age_hours). The BOUNDS are
// doctrine and live here, so neither a setting nor a tampered local config can
// widen it past the ceiling or disable it. Forward-only is not a knob; the
// existence of a fail-closed ceiling is fixed, only its length is tunable.
const (
	defaultStaleKeyMaxAge = 7 * 24 * time.Hour  // policy default; mirrors the server-side setting default
	minStaleKeyMaxAge     = 1 * time.Hour       // doctrinal floor — the window is always bounded
	maxStaleKeyMaxAge     = 30 * 24 * time.Hour // doctrinal ceiling — forward-only cap, cannot be exceeded
)

// staleKeyMaxAgeNanos holds the active window. Written by SetStaleKeyMaxAge from
// the config-refresh goroutine, read on the command-verify path — atomic so the
// two don't race. Zero means "unset": staleKeyMaxAge() falls back to the default.
var staleKeyMaxAgeNanos atomic.Int64

// staleKeyMaxAge returns the active stale-cache window.
func staleKeyMaxAge() time.Duration {
	if n := staleKeyMaxAgeNanos.Load(); n > 0 {
		return time.Duration(n)
	}
	return defaultStaleKeyMaxAge
}

// SetStaleKeyMaxAge applies an operator-configured window (in hours), clamped to
// the doctrinal [min,max] range. hours <= 0 means "unset" and restores the
// default. The clamp is the enforcement point: a setting or local config that
// asks for more than the ceiling gets the ceiling, never the request. Returns
// the window actually applied.
func SetStaleKeyMaxAge(hours int) time.Duration {
	d := defaultStaleKeyMaxAge
	if hours > 0 {
		d = time.Duration(hours) * time.Hour
		if d < minStaleKeyMaxAge {
			d = minStaleKeyMaxAge
		}
		if d > maxStaleKeyMaxAge {
			d = maxStaleKeyMaxAge
		}
	}
	staleKeyMaxAgeNanos.Store(int64(d))
	return d
}

// getPublicKeyDir returns the platform-specific directory for key cache files
// Uses constants package to ensure consistency with other path definitions.
func getPublicKeyDir() string {
	return constants.GetServerPublicKeyDir()
}

// getPrimaryKeyPath returns the path for the primary cached public key
// Uses constants package as single source of truth (BUG-012 fix).
func getPrimaryKeyPath() string {
	return constants.GetServerPublicKeyPath()
}

// getKeyPathByID returns the path for a specific key cached by key_id
func getKeyPathByID(keyID string) string {
	return filepath.Join(getPublicKeyDir(), "server_public_key_"+keyID)
}

// getPrimaryMetaPath returns the metadata file path for the primary key
func getPrimaryMetaPath() string {
	return filepath.Join(getPublicKeyDir(), "server_public_key.meta")
}

// CacheMetadata holds metadata about the cached public key
type CacheMetadata struct {
	KeyID    string    `json:"key_id"`
	Version  int       `json:"version"`
	CachedAt time.Time `json:"cached_at"`
	TTLHours int       `json:"ttl_hours"`
}

// IsExpired returns true if the cache TTL has been exceeded
func (m *CacheMetadata) IsExpired() bool {
	ttl := time.Duration(m.TTLHours) * time.Hour
	if ttl <= 0 {
		ttl = defaultCacheTTLHours * time.Hour
	}
	return time.Since(m.CachedAt) > ttl
}

// PublicKeyResponse represents the server's public key response
type PublicKeyResponse struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
	KeySize     int    `json:"key_size"`
	KeyID       string `json:"key_id"`
	Version     int    `json:"version"`
}

// ActivePublicKeyEntry represents one entry from GET /api/v1/public-keys
type ActivePublicKeyEntry struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
	IsPrimary bool   `json:"is_primary"`
	Version   int    `json:"version"`
	Algorithm string `json:"algorithm"`
}

// loadCacheMetadata loads the metadata sidecar file for the primary key
func loadCacheMetadata() (*CacheMetadata, error) {
	data, err := os.ReadFile(getPrimaryMetaPath())
	if err != nil {
		return nil, err
	}
	var meta CacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// saveCacheMetadata writes the metadata sidecar file
func saveCacheMetadata(meta *CacheMetadata) error {
	dir := getPublicKeyDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create key dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	return os.WriteFile(getPrimaryMetaPath(), data, 0644)
}

// FetchAndCacheServerPublicKey fetches the server's Ed25519 primary public key.
// Uses a TTL+key_id cache: skips the fetch only if both TTL is valid AND key_id matches.
// Implements Trust-On-First-Use (TOFU) with rotation awareness.
func FetchAndCacheServerPublicKey(serverURL string) (ed25519.PublicKey, error) {
	// Check if cache is still valid
	if meta, err := loadCacheMetadata(); err == nil && meta.KeyID != "" && !meta.IsExpired() {
		// Cache metadata is valid and within TTL — try to load the cached key
		if cachedKey, err := LoadCachedPublicKey(); err == nil && cachedKey != nil {
			return cachedKey, nil
		}
		// Cache file missing despite valid metadata — fall through to re-fetch
	}

	// Fetch primary key from server
	resp, err := http.Get(serverURL + "/api/v1/public-key")
	if err != nil {
		// Network failed — serve the cached key only within the bounded staleness
		// window (SEC-028). Past the ceiling, or when the cache age can't be
		// established, fail closed: forward-only forbids trusting a possibly
		// rotated-out key indefinitely.
		cachedKey, loadErr := LoadCachedPublicKey()
		if loadErr != nil {
			return nil, fmt.Errorf("failed to fetch public key from server: %w", err)
		}
		meta, metaErr := loadCacheMetadata()
		if metaErr != nil {
			return nil, fmt.Errorf("public key fetch failed and cache age is unknown (no metadata); refusing stale key: %w", err)
		}
		age := time.Since(meta.CachedAt)
		window := staleKeyMaxAge()
		if age > window {
			return nil, fmt.Errorf("public key fetch failed and cached key is stale (age %s > max %s); refusing: %w",
				age.Round(time.Hour), window, err)
		}
		// Degraded trust, not a routine warning: surface at ERROR so an operator
		// sees an agent running on an un-refreshed signing key.
		if teeLogger != nil {
			teeLogger.Error("agent", "crypto", "pubkey", "server unreachable, serving stale cached public key within bounded window", map[string]interface{}{
				"error":     err.Error(),
				"cache_age": age.Round(time.Minute).String(),
				"max_stale": window.String(),
				"key_id":    meta.KeyID,
			})
		}
		return cachedKey, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var keyResp PublicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
		return nil, fmt.Errorf("failed to parse public key response: %w", err)
	}

	if keyResp.Algorithm != "ed25519" {
		return nil, fmt.Errorf("unsupported signature algorithm: %s (expected ed25519)", keyResp.Algorithm)
	}

	pubKeyBytes, err := hex.DecodeString(keyResp.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key format: %w", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: expected %d bytes, got %d", ed25519.PublicKeySize, len(pubKeyBytes))
	}

	publicKey := ed25519.PublicKey(pubKeyBytes)

	// Cache the primary key
	if err := cachePublicKey(publicKey); err != nil {
		if teeLogger != nil {
			teeLogger.Warning("agent", "crypto", "pubkey", "failed to cache primary public key", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	// Use key_id from response (fall back to fingerprint for old servers)
	keyID := keyResp.KeyID
	if keyID == "" {
		keyID = keyResp.Fingerprint
	}

	// Write metadata sidecar
	meta := &CacheMetadata{
		KeyID:    keyID,
		Version:  keyResp.Version,
		CachedAt: time.Now().UTC(),
		TTLHours: defaultCacheTTLHours,
	}
	if err := saveCacheMetadata(meta); err != nil {
		if teeLogger != nil {
			teeLogger.Warning("agent", "crypto", "pubkey", "failed to save key cache metadata", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	// Also cache by key_id for multi-key lookup
	if keyID != "" {
		if err := CachePublicKeyByID(keyID, publicKey); err != nil {
			if teeLogger != nil {
				teeLogger.Warning("agent", "crypto", "pubkey", "failed to cache key by ID", map[string]interface{}{
					"key_id": keyID,
					"error":  err.Error(),
				})
			}
		}
	}

	if teeLogger != nil {
		teeLogger.Info("agent", "crypto", "pubkey", "public key fetched and cached", map[string]interface{}{
			"key_id":  keyID,
			"version": keyResp.Version,
		})
	}
	return publicKey, nil
}

// FetchAndCacheAllActiveKeys fetches all active public keys from GET /api/v1/public-keys
// and caches each one by its key_id. Used during key rotation transition windows.
func FetchAndCacheAllActiveKeys(serverURL string) ([]ActivePublicKeyEntry, error) {
	resp, err := http.Get(serverURL + "/api/v1/public-keys")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active public keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var entries []ActivePublicKeyEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode public keys list: %w", err)
	}

	for _, entry := range entries {
		if entry.Algorithm != "ed25519" {
			continue
		}
		pubKeyBytes, err := hex.DecodeString(entry.PublicKey)
		if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
			continue
		}
		if err := CachePublicKeyByID(entry.KeyID, ed25519.PublicKey(pubKeyBytes)); err != nil {
			if teeLogger != nil {
				teeLogger.Warning("agent", "crypto", "pubkey", "failed to cache key", map[string]interface{}{
					"key_id": entry.KeyID,
					"error":  err.Error(),
				})
			}
		}
	}

	return entries, nil
}

// LoadCachedPublicKey loads the primary cached public key from disk (backward compat path)
func LoadCachedPublicKey() (ed25519.PublicKey, error) {
	data, err := os.ReadFile(getPrimaryKeyPath())
	if err != nil {
		return nil, err
	}
	if len(data) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("cached public key has invalid size: %d bytes", len(data))
	}
	return ed25519.PublicKey(data), nil
}

// LoadCachedPublicKeyByID loads a cached public key by its key_id.
// Falls back to the primary key if the key_id-specific file does not exist.
func LoadCachedPublicKeyByID(keyID string) (ed25519.PublicKey, error) {
	if keyID == "" {
		return LoadCachedPublicKey()
	}
	data, err := os.ReadFile(getKeyPathByID(keyID))
	if err == nil && len(data) == ed25519.PublicKeySize {
		return ed25519.PublicKey(data), nil
	}
	// Fall back to primary
	return LoadCachedPublicKey()
}

// IsKeyIDCached returns true if a key with the given key_id is cached locally
func IsKeyIDCached(keyID string) bool {
	if keyID == "" {
		return false
	}
	info, err := os.Stat(getKeyPathByID(keyID))
	return err == nil && info.Size() == ed25519.PublicKeySize
}

// cachePublicKey saves the primary public key to disk (backward compat path)
func cachePublicKey(publicKey ed25519.PublicKey) error {
	dir := getPublicKeyDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return os.WriteFile(getPrimaryKeyPath(), publicKey, 0644)
}

// CachePublicKeyByID saves a public key under its key_id filename
func CachePublicKeyByID(keyID string, publicKey ed25519.PublicKey) error {
	if keyID == "" {
		return fmt.Errorf("keyID cannot be empty")
	}
	dir := getPublicKeyDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return os.WriteFile(getKeyPathByID(keyID), publicKey, 0644)
}

// GetPublicKey returns the primary cached public key or fetches it from the server
func GetPublicKey(serverURL string) (ed25519.PublicKey, error) {
	// Try with TTL-aware fetch (will use cache if valid)
	return FetchAndCacheServerPublicKey(serverURL)
}
