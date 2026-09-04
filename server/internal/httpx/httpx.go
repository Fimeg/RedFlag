// Package httpx provides shared HTTP client helpers for server-side outbound
// calls. It centralises transport tuning so that hot clients (OSV.dev, npm,
// PyPI, GitHub, Repology, etc.) share a consistent idle-connection pool
// instead of each inheriting http.DefaultTransport's MaxIdleConnsPerHost=2,
// which causes constant connection churn under load.
package httpx

import (
	"net/http"
	"time"
)

// NewClient returns an *http.Client with the given timeout and a transport
// tuned for repeated calls to a small set of hot API hosts. The transport is
// cloned from http.DefaultTransport so Proxy, TLS settings, and all other
// defaults are inherited unchanged; only the connection-pool limits are
// tightened.
//
// Chosen pool values:
//
//	MaxIdleConns=100         — global cap; headroom for many hosts without
//	                           unbounded growth.
//	MaxIdleConnsPerHost=10   — per-host cap; enough to absorb bursts to
//	                           OSV.dev / npm / PyPI without leaving thousands
//	                           of idle FDs open.
//	IdleConnTimeout=90s      — matches http.DefaultTransport's own default so
//	                           behaviour is predictable on firewalls that close
//	                           idle connections after ~60–120 s.
func NewClient(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = 90 * time.Second
	return &http.Client{
		Timeout:   timeout,
		Transport: t,
	}
}
