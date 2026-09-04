package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// HashCache provides LRU caching for expected package hashes
type HashCache struct {
	mu         sync.RWMutex
	hashes     map[string]string
	maxSize    int
	evicted    map[string]time.Time
}

// NewHashCache creates a new hash cache
func NewHashCache(maxSize int) *HashCache {
	return &HashCache{
		hashes:     make(map[string]string),
		maxSize:    maxSize,
		evicted:    make(map[string]time.Time),
	}
}

// key creates a cache key from package type and name
func (c *HashCache) key(packageType, packageName, version string) string {
	return fmt.Sprintf("%s:%s:%s", packageType, packageName, version)
}

// Get retrieves a cached hash
func (c *HashCache) Get(packageType, packageName, version string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.key(packageType, packageName, version)
	hash, ok := c.hashes[key]
	return hash, ok
}

// Set stores a hash in the cache
func (c *HashCache) Set(packageType, packageName, version, hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.key(packageType, packageName, version)

	// Remove old entry if exists
	if _, ok := c.hashes[key]; ok {
		c.evict(key)
	}

	c.hashes[key] = hash
	c.touch(key)

	// Enforce size limit
	for len(c.hashes) > c.maxSize {
		c.evictOldest()
	}
}

// Delete removes a hash from the cache
func (c *HashCache) Delete(packageType, packageName, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.key(packageType, packageName, version)
	delete(c.hashes, key)
}

// evict removes a key from the cache
func (c *HashCache) evict(key string) {
	delete(c.hashes, key)
	delete(c.evicted, key)
}

// touch updates the access time for a key
func (c *HashCache) touch(key string) {
	c.evicted[key] = time.Now()
}

// evictOldest removes the oldest accessed key when cache is full
func (c *HashCache) evictOldest() {
	var oldest string
	var oldestTime time.Time

	c.mu.RLock()
	for key, t := range c.evicted {
		if oldest == "" || t.Before(oldestTime) {
			oldest = key
			oldestTime = t
		}
	}
	c.mu.RUnlock()

	if oldest != "" {
		c.evict(oldest)
	}
}

// VerifyHashFromCache downloads a package only if not cached, verifies hash, and caches result
func (c *HashCache) VerifyHashFromCache(packageType, packageName, version, expectedSHA256 string) error {
	if expectedSHA256 == "" {
		return nil
	}

	// Check cache first
	if cachedHash, cached := c.Get(packageType, packageName, version); cached {
		if cachedHash == expectedSHA256 {
			return nil // Hash verified from cache
		}
		// Cache has different hash - re-download to update cache
	}

	// Download and compute hash
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/downloads/artifact?ecosystem=%s&package_name=%s&version=%s",
		getDownloaderURL(), packageType, packageName, version))
	if err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Compute hash while streaming
	h := sha256.New()
	_, err = io.Copy(h, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read package: %w", err)
	}

	computedSHA256 := hex.EncodeToString(h.Sum(nil))

	// Verify against expected
	if computedSHA256 != expectedSHA256 {
		return fmt.Errorf("package hash mismatch: expected %s, got %s", expectedSHA256, computedSHA256)
	}

	// Cache the verified hash
	c.Set(packageType, packageName, version, computedSHA256)

	return nil
}

// Clear evicts all cached hashes
func (c *HashCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashes = make(map[string]string)
	c.evicted = make(map[string]time.Time)
}

// Size returns current cache size
func (c *HashCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.hashes)
}

// getDownloaderURL returns the URL of the download handler
// This is a simplified version - in production, use the download handler's getServerURL method
func getDownloaderURL() string {
	// Default to localhost for local testing
	// In production, this would come from config
	return "http://localhost:8080"
}
