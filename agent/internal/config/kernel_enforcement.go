package config

import (
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

// KernelEnforcementConfig holds configuration for kernel-level enforcement
type KernelEnforcementConfig struct {
	// Enable kernel enforcement (eBPF on Linux, WDAC on Windows)
	Enabled bool `json:"enabled" env:"REDFLAG_KERNEL_ENFORCEMENT_ENABLED" default:"true"`

	// Ring buffer path for eBPF events (Linux only)
	RingBufferPath string `json:"ring_buffer_path" env:"REDFLAG_RING_BUFFER_PATH" default:"/var/run/redflag/ebpf-ring"`

	// Socket path for rs-helper communication
	RsHelperSocket string `json:"rs_helper_socket" env:"REDFLAG_RS_HELPER_SOCKET" default:"/var/run/redflag/rs-helper.sock"`

	// Policy check timeout
	PolicyCheckTimeout time.Duration `json:"policy_check_timeout" env:"REDFLAG_POLICY_CHECK_TIMEOUT" default:"10s"`

	// Fail-closed mode (deny on kernel enforcement failure)
	FailClosed bool `json:"fail_closed" env:"REDFLAG_KERNEL_ENFORCEMENT_FAIL_CLOSED" default:"true"`
}

// GetDefaultKernelEnforcementConfig returns default kernel enforcement configuration
func GetDefaultKernelEnforcementConfig() KernelEnforcementConfig {
	return KernelEnforcementConfig{
		Enabled:             true,
		RingBufferPath:      constants.GetAgentStateDir() + "/ebpf-ring",
		RsHelperSocket:      "/var/run/redflag/rs-helper.sock",
		PolicyCheckTimeout:  10 * time.Second,
		FailClosed:          true,
	}
}

// MergeKernelEnforcement merges kernel enforcement config from source into target
func MergeKernelEnforcement(target, source KernelEnforcementConfig) {
	if source.Enabled {
		target.Enabled = source.Enabled
	}
	if source.RingBufferPath != "" {
		target.RingBufferPath = source.RingBufferPath
	}
	if source.RsHelperSocket != "" {
		target.RsHelperSocket = source.RsHelperSocket
	}
	if source.PolicyCheckTimeout > 0 {
		target.PolicyCheckTimeout = source.PolicyCheckTimeout
	}
	if source.FailClosed {
		target.FailClosed = source.FailClosed
	}
}
