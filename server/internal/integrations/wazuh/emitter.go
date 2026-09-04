// Package wazuh emits RedFlag security events to a local Wazuh agent's queue
// socket in ECS format (INTEG-001). Outbound-only: this opens no listener and
// is unrelated to the pull-only agent<->server channel. RedFlag's own journal
// remains the source of truth; Wazuh is a mirror — writes are best-effort,
// never block the caller, and drops are counted and logged.
package wazuh

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
)

// DefaultSocketPath is where a local Wazuh agent or manager listens.
const DefaultSocketPath = "/var/ossec/queue/sockets/queue"

// framePrefix is the Wazuh queue protocol header: <queue>:<location>:
// 1 = ossec message queue, "redflag" is the location tag decoders key on.
const framePrefix = "1:redflag:"

const writeDeadline = 250 * time.Millisecond

// ruleIDs maps RedFlag security event types to Wazuh custom rule IDs
// (999xxx user range). Kept in lockstep with docs/wazuh-ruleset.xml.
var ruleIDs = map[string]int{
	models.SecurityEventTypes.CmdSignatureVerificationFailed:    999002,
	models.SecurityEventTypes.UpdateNonceInvalid:                999003,
	models.SecurityEventTypes.UpdateSignatureVerificationFailed: 999004,
	models.SecurityEventTypes.MachineIDMismatch:                 999005,
	models.SecurityEventTypes.AuthJWTValidationFailed:           999006,
	models.SecurityEventTypes.AgentRegistrationFailed:           999007,
	models.SecurityEventTypes.UnauthorizedAccessAttempt:         999008,
	models.SecurityEventTypes.ConfigTamperingDetected:           999009,
	models.SecurityEventTypes.AnomalousBehavior:                 999010,
	models.SecurityEventTypes.CmdSigned:                         999011,
	models.SecurityEventTypes.CmdSignatureVerificationSuccess:   999012,
}

const genericRuleID = 999001

// Emitter writes ECS-formatted events to the Wazuh queue socket. Connection
// is lazy and re-established once per emit on failure; beyond that the event
// is dropped, counted, and logged (rate-limited) — never blocking.
type Emitter struct {
	socketPath string

	mu   sync.Mutex
	conn net.Conn

	dropped     atomic.Uint64
	lastDropLog atomic.Int64 // unix seconds, rate-limits drop warnings
}

// New returns an Emitter for the given socket path ("" = DefaultSocketPath).
// No connection is attempted until the first Emit.
func New(socketPath string) *Emitter {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	return &Emitter{socketPath: socketPath}
}

// Dropped reports how many events were lost to socket failures.
func (e *Emitter) Dropped() uint64 { return e.dropped.Load() }

// Emit formats the event as ECS JSON and writes one datagram. Failures drop
// the event after a single reconnect attempt. Safe for concurrent use.
func (e *Emitter) Emit(ev *models.SecurityEvent) {
	payload, err := json.Marshal(toECS(ev))
	if err != nil {
		e.drop(fmt.Sprintf("marshal failed: %v", err))
		return
	}
	frame := append([]byte(framePrefix), payload...)

	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.writeLocked(frame); err != nil {
		// One reconnect attempt: the Wazuh agent may have restarted.
		e.closeLocked()
		if err := e.writeLocked(frame); err != nil {
			e.drop(fmt.Sprintf("socket write failed: %v", err))
		}
	}
}

func (e *Emitter) writeLocked(frame []byte) error {
	if e.conn == nil {
		conn, err := net.Dial("unixgram", e.socketPath)
		if err != nil {
			return err
		}
		e.conn = conn
	}
	if err := e.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}
	_, err := e.conn.Write(frame)
	return err
}

func (e *Emitter) closeLocked() {
	if e.conn != nil {
		if err := e.conn.Close(); err != nil {
			log.Printf("[WARN] [server] [wazuh-emitter] close failed: %v", err)
		}
		e.conn = nil
	}
}

// drop counts a lost event and logs at most once per minute so a dead socket
// cannot flood the server log while still leaving history (ETHOS #1).
func (e *Emitter) drop(reason string) {
	n := e.dropped.Add(1)
	now := time.Now().Unix()
	last := e.lastDropLog.Load()
	if now-last >= 60 && e.lastDropLog.CompareAndSwap(last, now) {
		log.Printf("[WARN] [server] [wazuh-emitter] event dropped (%d total): %s", n, reason)
	}
}

// ecsEvent is the Wazuh-ingestible ECS shape (see INTEG-001 spec).
type ecsEvent struct {
	Timestamp string                 `json:"@timestamp"`
	Event     ecsEventBlock          `json:"event"`
	Rule      ecsRule                `json:"rule"`
	Agent     *ecsAgent              `json:"agent,omitempty"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"redflag,omitempty"`
	Wazuh     ecsWazuh               `json:"wazuh"`
}

type ecsEventBlock struct {
	Kind     string   `json:"kind"`
	Category []string `json:"category"`
	Type     []string `json:"type"`
	Module   string   `json:"module"`
	Action   string   `json:"action"`
	Outcome  string   `json:"outcome"`
	Severity int      `json:"severity"`
}

type ecsRule struct {
	ID          string `json:"id"`
	Level       int    `json:"level"`
	Description string `json:"description"`
}

type ecsAgent struct {
	ID string `json:"id"`
}

type ecsWazuh struct {
	Integration ecsIntegration `json:"integration"`
}

type ecsIntegration struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Decoders []string `json:"decoders"`
	Rules    []string `json:"rules"`
}

func toECS(ev *models.SecurityEvent) ecsEvent {
	ruleID, ok := ruleIDs[ev.EventType]
	if !ok {
		ruleID = genericRuleID
	}
	severity, level := severityFor(ev.Level)
	outcome := "failure"
	switch ev.EventType {
	case models.SecurityEventTypes.CmdSigned,
		models.SecurityEventTypes.CmdSignatureVerificationSuccess:
		outcome = "success"
	}

	var agent *ecsAgent
	if id := ev.AgentID.String(); id != "00000000-0000-0000-0000-000000000000" {
		agent = &ecsAgent{ID: id}
	}

	return ecsEvent{
		Timestamp: ev.Timestamp.UTC().Format(time.RFC3339),
		Event: ecsEventBlock{
			Kind:     "alert",
			Category: []string{"security"},
			Type:     []string{"info"},
			Module:   "redflag",
			Action:   ev.EventType,
			Outcome:  outcome,
			Severity: severity,
		},
		Rule: ecsRule{
			ID:          fmt.Sprintf("%d", ruleID),
			Level:       level,
			Description: ev.Message,
		},
		Agent:   agent,
		Message: ev.Message,
		Details: ev.Details,
		Wazuh: ecsWazuh{
			Integration: ecsIntegration{
				Name:     "redflag",
				Category: "security",
				Decoders: []string{"json"},
				Rules:    []string{fmt.Sprintf("%d", ruleID)},
			},
		},
	}
}

// severityFor maps RedFlag levels to (ECS event.severity, Wazuh rule level).
func severityFor(level string) (int, int) {
	switch level {
	case "CRITICAL":
		return 9, 12
	case "WARNING":
		return 5, 7
	case "INFO":
		return 3, 3
	default:
		return 1, 1
	}
}
