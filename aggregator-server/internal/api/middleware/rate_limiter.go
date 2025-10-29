package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
	Enabled  bool          `json:"enabled"`
}

// RateLimitEntry tracks requests for a specific key
type RateLimitEntry struct {
	Requests []time.Time
	mutex    sync.RWMutex
}

// RateLimiter implements in-memory rate limiting with user-configurable settings
type RateLimiter struct {
	entries sync.Map // map[string]*RateLimitEntry
	configs map[string]RateLimitConfig
	mutex   sync.RWMutex
}

// RateLimitSettings holds all user-configurable rate limit settings
type RateLimitSettings struct {
	AgentRegistration RateLimitConfig `json:"agent_registration"`
	AgentCheckIn      RateLimitConfig `json:"agent_checkin"`
	AgentReports      RateLimitConfig `json:"agent_reports"`
	AdminTokenGen      RateLimitConfig `json:"admin_token_generation"`
	AdminOperations    RateLimitConfig `json:"admin_operations"`
	PublicAccess       RateLimitConfig `json:"public_access"`
}

// DefaultRateLimitSettings provides sensible defaults
func DefaultRateLimitSettings() RateLimitSettings {
	return RateLimitSettings{
		AgentRegistration: RateLimitConfig{
			Requests: 5,
			Window:   time.Minute,
			Enabled:  true,
		},
		AgentCheckIn: RateLimitConfig{
			Requests: 60,
			Window:   time.Minute,
			Enabled:  true,
		},
		AgentReports: RateLimitConfig{
			Requests: 30,
			Window:   time.Minute,
			Enabled:  true,
		},
		AdminTokenGen: RateLimitConfig{
			Requests: 10,
			Window:   time.Minute,
			Enabled:  true,
		},
		AdminOperations: RateLimitConfig{
			Requests: 100,
			Window:   time.Minute,
			Enabled:  true,
		},
		PublicAccess: RateLimitConfig{
			Requests: 20,
			Window:   time.Minute,
			Enabled:  true,
		},
	}
}

// NewRateLimiter creates a new rate limiter with default settings
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		entries: sync.Map{},
	}

	// Load default settings
	defaults := DefaultRateLimitSettings()
	rl.UpdateSettings(defaults)

	return rl
}

// UpdateSettings updates rate limit configurations
func (rl *RateLimiter) UpdateSettings(settings RateLimitSettings) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.configs = map[string]RateLimitConfig{
		"agent_registration": settings.AgentRegistration,
		"agent_checkin":      settings.AgentCheckIn,
		"agent_reports":      settings.AgentReports,
		"admin_token_gen":     settings.AdminTokenGen,
		"admin_operations":   settings.AdminOperations,
		"public_access":       settings.PublicAccess,
	}
}

// GetSettings returns current rate limit settings
func (rl *RateLimiter) GetSettings() RateLimitSettings {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	return RateLimitSettings{
		AgentRegistration: rl.configs["agent_registration"],
		AgentCheckIn:      rl.configs["agent_checkin"],
		AgentReports:      rl.configs["agent_reports"],
		AdminTokenGen:      rl.configs["admin_token_gen"],
		AdminOperations:    rl.configs["admin_operations"],
		PublicAccess:       rl.configs["public_access"],
	}
}

// RateLimit creates middleware for a specific rate limit type
func (rl *RateLimiter) RateLimit(limitType string, keyFunc func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rl.mutex.RLock()
		config, exists := rl.configs[limitType]
		rl.mutex.RUnlock()

		if !exists || !config.Enabled {
			c.Next()
			return
		}

		key := keyFunc(c)
		if key == "" {
			c.Next()
			return
		}

		// Check rate limit
		allowed, resetTime := rl.checkRateLimit(key, config)
		if !allowed {
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.Requests))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
			c.Header("Retry-After", fmt.Sprintf("%d", int(resetTime.Sub(time.Now()).Seconds())))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
				"limit": config.Requests,
				"window": config.Window.String(),
				"reset_time": resetTime,
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		remaining := rl.getRemainingRequests(key, config)
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", config.Requests))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(config.Window).Unix()))

		c.Next()
	}
}

// checkRateLimit checks if the request is allowed
func (rl *RateLimiter) checkRateLimit(key string, config RateLimitConfig) (bool, time.Time) {
	now := time.Now()

	// Get or create entry
	entryInterface, _ := rl.entries.LoadOrStore(key, &RateLimitEntry{
		Requests: []time.Time{},
	})
	entry := entryInterface.(*RateLimitEntry)

	entry.mutex.Lock()
	defer entry.mutex.Unlock()

	// Clean old requests outside the window
	cutoff := now.Add(-config.Window)
	validRequests := make([]time.Time, 0)
	for _, reqTime := range entry.Requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}

	// Check if under limit
	if len(validRequests) >= config.Requests {
		// Find when the oldest request expires
		oldestRequest := validRequests[0]
		resetTime := oldestRequest.Add(config.Window)
		return false, resetTime
	}

	// Add current request
	entry.Requests = append(validRequests, now)

	// Clean up expired entries periodically
	if len(entry.Requests) == 0 {
		rl.entries.Delete(key)
	}

	return true, time.Time{}
}

// getRemainingRequests calculates remaining requests for the key
func (rl *RateLimiter) getRemainingRequests(key string, config RateLimitConfig) int {
	entryInterface, ok := rl.entries.Load(key)
	if !ok {
		return config.Requests
	}

	entry := entryInterface.(*RateLimitEntry)
	entry.mutex.RLock()
	defer entry.mutex.RUnlock()

	now := time.Now()
	cutoff := now.Add(-config.Window)
	count := 0

	for _, reqTime := range entry.Requests {
		if reqTime.After(cutoff) {
			count++
		}
	}

	remaining := config.Requests - count
	if remaining < 0 {
		remaining = 0
	}

	return remaining
}

// CleanupExpiredEntries removes expired entries to prevent memory leaks
func (rl *RateLimiter) CleanupExpiredEntries() {
	rl.entries.Range(func(key, value interface{}) bool {
		entry := value.(*RateLimitEntry)
		entry.mutex.Lock()

		now := time.Now()
		validRequests := make([]time.Time, 0)
		for _, reqTime := range entry.Requests {
			if reqTime.After(now.Add(-time.Hour)) { // Keep requests from last hour
				validRequests = append(validRequests, reqTime)
			}
		}

		if len(validRequests) == 0 {
			rl.entries.Delete(key)
		} else {
			entry.Requests = validRequests
		}

		entry.mutex.Unlock()
		return true
	})
}

// Key generation functions
func KeyByIP(c *gin.Context) string {
	return c.ClientIP()
}

func KeyByAgentID(c *gin.Context) string {
	return c.Param("id")
}

func KeyByUserID(c *gin.Context) string {
	// This would extract user ID from JWT or session
	// For now, use IP as fallback
	return c.ClientIP()
}

func KeyByIPAndPath(c *gin.Context) string {
	return c.ClientIP() + ":" + c.Request.URL.Path
}