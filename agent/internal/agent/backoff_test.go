package agent

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
)

// TestClassifyFailure locks in the BUG-014 policy: dead credentials and
// machine-binding mismatches are terminal; everything else is transient.
func TestClassifyFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want failureClass
	}{
		{"machine mismatch", client.ErrMachineMismatch, failureTerminal},
		{"refresh token invalid", client.ErrRefreshTokenInvalid, failureTerminal},
		{"wrapped machine mismatch", fmt.Errorf("get commands: %w", client.ErrMachineMismatch), failureTerminal},
		{"wrapped refresh invalid", fmt.Errorf("renew: %w", client.ErrRefreshTokenInvalid), failureTerminal},
		{"unauthorized alone is not terminal (renewal may fix it)", client.ErrUnauthorized, failureTransient},
		{"plain network error", errors.New("dial tcp: connection refused"), failureTransient},
		{"nil-adjacent generic error", errors.New("502 bad gateway"), failureTransient},
	}
	for _, tc := range cases {
		if got := classifyFailure(tc.err); got != tc.want {
			t.Errorf("%s: classifyFailure() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestDelayForFailure verifies the policy curves: terminal is a long flat
// delay independent of attempt count; transient follows the jittered
// exponential bounded by base and max.
func TestDelayForFailure(t *testing.T) {
	base := 5 * time.Second
	max := 5 * time.Minute

	for _, attempt := range []int{1, 3, 50} {
		if got := delayForFailure(failureTerminal, attempt, base, max); got != terminalRetryDelay {
			t.Errorf("terminal attempt %d: delay = %s, want flat %s", attempt, got, terminalRetryDelay)
		}
	}

	for attempt := 1; attempt <= 30; attempt++ {
		got := delayForFailure(failureTransient, attempt, base, max)
		if got < base || got > max {
			t.Errorf("transient attempt %d: delay %s outside [%s, %s]", attempt, got, base, max)
		}
	}
}
