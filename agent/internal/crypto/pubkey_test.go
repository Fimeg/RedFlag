package crypto

import (
	"testing"
	"time"
)

func TestCacheMetadataIsExpired(t *testing.T) {
	tests := []struct {
		name     string
		meta     CacheMetadata
		expected bool
	}{
		{
			name:     "fresh_within_ttl",
			meta:     CacheMetadata{CachedAt: time.Now(), TTLHours: 24},
			expected: false,
		},
		{
			name:     "expired_past_ttl",
			meta:     CacheMetadata{CachedAt: time.Now().Add(-25 * time.Hour), TTLHours: 24},
			expected: true,
		},
		{
			name:     "zero_ttl_defaults_24h_fresh",
			meta:     CacheMetadata{CachedAt: time.Now(), TTLHours: 0},
			expected: false,
		},
		{
			name:     "zero_ttl_defaults_24h_expired",
			meta:     CacheMetadata{CachedAt: time.Now().Add(-25 * time.Hour), TTLHours: 0},
			expected: true,
		},
		{
			name:     "exactly_at_ttl_boundary",
			meta:     CacheMetadata{CachedAt: time.Now().Add(-24 * time.Hour), TTLHours: 24},
			expected: true, // at exactly TTL, treat as expired
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.meta.IsExpired()
			if got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v (cachedAt=%v, ttl=%dh)",
					got, tt.expected, tt.meta.CachedAt, tt.meta.TTLHours)
			}
		})
	}
}
