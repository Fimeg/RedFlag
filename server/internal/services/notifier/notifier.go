// Package notifier dispatches system events to external notification sinks
// (ntfy, SMTP email). Best-effort, fail-open — the dashboard is the source
// of truth; notifications are a courtesy tap on the shoulder.
//
// Design of record: docs/tasks/NOTIFY-001-notification-system.md
package notifier

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// SettingsReader is the subset of SecuritySettingsService that the
// notifier needs. Implemented by the services package; avoids a
// circular import.
type SettingsReader interface {
	GetSetting(category, key string) (interface{}, error)
}

// Setup builds and registers sinks on a Dispatcher from security_settings.
// Pass the dispatcher and a settings reader (typically SecuritySettingsService).
// Returns the count of sinks registered.
func Setup(d *Dispatcher, settings SettingsReader) int {
	count := 0

	// --- ntfy ---
	if url := getString(settings, "notify", "ntfy_url"); url != "" {
		classes := ParseEventClasses(getString(settings, "notify", "ntfy_events"))
		severity := getString(settings, "notify", "ntfy_severity")
		if severity == "" {
			severity = "error,critical"
		}
		sink := NewNtfySink(url, severity, classes)
		d.Register(sink, classes...)
		log.Printf("[INFO] [notifier] ntfy_configured url=%s severity=%s classes=%v",
			url, severity, classes)
		count++
	}

	// --- SMTP ---
	if host := getString(settings, "notify", "smtp_host"); host != "" {
		from := getString(settings, "notify", "smtp_from")
		to := getString(settings, "notify", "smtp_to")
		if from != "" && to != "" {
			classes := ParseEventClasses(getString(settings, "notify", "smtp_events"))
			severity := getString(settings, "notify", "smtp_severity")
			if severity == "" {
				severity = "error,critical"
			}
			sink := NewSmtpSink(host, from, to, severity, classes)
			d.Register(sink, classes...)
			log.Printf("[INFO] [notifier] smtp_configured host=%s to=%s severity=%s classes=%v",
				host, to, severity, classes)
			count++
		} else {
			log.Printf("[WARN] [notifier] smtp_host_configured_but_missing_from_or_to host=%s", host)
		}
	}

	return count
}

// getString reads a string-valued setting with a default fallback.
func getString(settings SettingsReader, category, key string) string {
	v, err := settings.GetSetting(category, key)
	if err != nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// EventClass categorises a notification by what happened.
type EventClass string

const (
	ClassSecurity     EventClass = "security"
	ClassAgentOffline EventClass = "agent_offline"
	ClassUpdateFailed EventClass = "update_failed"
	ClassThreat       EventClass = "threat"
	ClassEOL          EventClass = "eol"
)

// NotifyEvent is the payload passed to every configured sink.
type NotifyEvent struct {
	Class       EventClass
	Severity    string // info, warning, error, critical
	Title       string
	Message     string
	AgentID     string // optional — empty for server-scoped events
	Fingerprint string // dedup key — same fingerprint within class suppresses repeats
}

// Sink is one external notification target.
type Sink interface {
	// ID returns a stable short name for logging (e.g. "ntfy", "smtp").
	ID() string
	// Send delivers the event. Must not panic. Returns error for logging.
	Send(ctx context.Context, event NotifyEvent) error
}

// Dispatcher routes events to configured sinks based on event class.
// Zero-value is usable (no sinks).
type Dispatcher struct {
	mu    sync.RWMutex
	sinks map[EventClass][]Sink

	// Dedup window — suppress repeated events with the same class+fingerprint
	// inside this window.
	dedupWindow time.Duration
	dedup       map[string]time.Time
	dedupMu     sync.Mutex
}

// NewDispatcher creates a Dispatcher with the given dedup window.
// A 5-minute window is reasonable for most deployments.
func NewDispatcher(dedupWindow time.Duration) *Dispatcher {
	return &Dispatcher{
		sinks:       make(map[EventClass][]Sink),
		dedupWindow: dedupWindow,
		dedup:       make(map[string]time.Time),
	}
}

// Register adds a sink for one or more event classes. Pass zero classes to
// subscribe to everything.
func (d *Dispatcher) Register(sink Sink, classes ...EventClass) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(classes) == 0 {
		classes = []EventClass{ClassSecurity, ClassAgentOffline, ClassUpdateFailed, ClassThreat, ClassEOL}
	}
	for _, c := range classes {
		d.sinks[c] = append(d.sinks[c], sink)
	}
}

// Dispatch sends the event to every sink subscribed to its class.
// Best-effort: individual sink failures are logged and do not block
// other sinks or the caller. Dedup suppresses events whose class+fingerprint
// have been seen within the dedup window.
func (d *Dispatcher) Dispatch(ctx context.Context, event NotifyEvent) {
	// Dedup check — default fingerprint is the message if none set.
	if event.Fingerprint == "" {
		event.Fingerprint = event.Message
	}
	d.dedupMu.Lock()
	key := fmt.Sprintf("%s|%s", event.Class, event.Fingerprint)
	if last, ok := d.dedup[key]; ok && time.Since(last) < d.dedupWindow {
		d.dedupMu.Unlock()
		return
	}
	d.dedup[key] = time.Now()
	// Prune expired entries so the map doesn't grow unbounded.
	for k, v := range d.dedup {
		if time.Since(v) > d.dedupWindow*2 {
			delete(d.dedup, k)
		}
	}
	d.dedupMu.Unlock()

	d.mu.RLock()
	sinks := d.sinks[event.Class]
	d.mu.RUnlock()

	for _, sink := range sinks {
		go func(s Sink) {
			if err := s.Send(ctx, event); err != nil {
				log.Printf("[WARN] [notifier] [%s] send_failed class=%s title=%q error=%v",
					s.ID(), event.Class, event.Title, err)
			}
		}(sink)
	}
}

// SinkIDs returns the IDs of all registered sinks for diagnostics.
func (d *Dispatcher) SinkIDs() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	seen := map[string]bool{}
	for _, sinks := range d.sinks {
		for _, s := range sinks {
			seen[s.ID()] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

// ParseEventClasses splits a comma-separated config string into event classes.
// Unknown tokens are dropped with a log warning.
func ParseEventClasses(raw string) []EventClass {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	classes := make([]EventClass, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		switch p {
		case "security":
			classes = append(classes, ClassSecurity)
		case "agent_offline", "agent-offline":
			classes = append(classes, ClassAgentOffline)
		case "update_failed", "update-failed":
			classes = append(classes, ClassUpdateFailed)
		case "threat":
			classes = append(classes, ClassThreat)
		case "eol":
			classes = append(classes, ClassEOL)
		default:
			log.Printf("[WARN] [notifier] unknown_event_class class=%q", p)
		}
	}
	return classes
}

// SeverityAll is the default — all severities pass through.
const SeverityAll = "info,warning,error,critical"

// SeverityPasses returns true when a given severity string meets a
// comma-separated minimum-severity filter. e.g. "error,critical" passes
// error and critical but not warning/info.
func SeverityPasses(filter, severity string) bool {
	if filter == "" || filter == SeverityAll {
		return true
	}
	for _, s := range strings.Split(filter, ",") {
		if strings.TrimSpace(s) == severity {
			return true
		}
	}
	return false
}
