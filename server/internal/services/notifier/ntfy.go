package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NtfySink delivers events to an ntfy server via HTTP POST.
// ntfy is a self-hostable pub-sub notification service — https://ntfy.sh.
type NtfySink struct {
	url        string
	severity   string   // comma-separated minimum severity
	eventClass []EventClass
	client     *http.Client
}

// NewNtfySink creates an ntfy sink. url is the full topic URL
// (e.g. "https://ntfy.example.com/redflag"). severity is the minimum
// severity filter ("error,critical" = only error+critical).
// eventClass is the parsed list of event classes to subscribe to.
func NewNtfySink(url, severity string, eventClass []EventClass) *NtfySink {
	return &NtfySink{
		url:        strings.TrimRight(url, "/"),
		severity:   severity,
		eventClass: eventClass,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *NtfySink) ID() string { return "ntfy" }

// ntfyPayload is the JSON body for an ntfy publish request.
type ntfyPayload struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags,omitempty"`
}

func (s *NtfySink) Send(ctx context.Context, event NotifyEvent) error {
	if !SeverityPasses(s.severity, event.Severity) {
		return nil
	}

	prio := ntfyPriority(event)
	payload := ntfyPayload{
		Title:    event.Title,
		Message:  event.Message,
		Priority: prio,
		Tags:     ntfyTags(event),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ntfy payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ntfy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}
	return nil
}

func ntfyPriority(event NotifyEvent) int {
	switch event.Class {
	case ClassSecurity:
		return 5 // max — immediate
	case ClassAgentOffline:
		return 4 // high
	case ClassUpdateFailed:
		return 4
	case ClassThreat:
		return 4
	case ClassEOL:
		return 3 // default
	default:
		return 3
	}
}

func ntfyTags(event NotifyEvent) []string {
	switch event.Class {
	case ClassSecurity:
		return []string{"lock", "redflag"}
	case ClassAgentOffline:
		return []string{"no_entry", "redflag"}
	case ClassUpdateFailed:
		return []string{"x", "redflag"}
	case ClassThreat:
		return []string{"warning", "redflag"}
	case ClassEOL:
		return []string{"hourglass", "redflag"}
	default:
		return []string{"redflag"}
	}
}
