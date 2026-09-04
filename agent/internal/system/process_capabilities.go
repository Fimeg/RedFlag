package system

import (
	"strconv"
	"strings"
)

// capabilityNames is indexed by capability bit. CAP_CHECKPOINT_RESTORE (40) is
// CAP_LAST_CAP on Linux 5.9 and later; a bit past the end of this table is
// reported as CAP_<n> rather than dropped, so a newer kernel stays legible.
var capabilityNames = []string{
	"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_DAC_READ_SEARCH", "CAP_FOWNER",
	"CAP_FSETID", "CAP_KILL", "CAP_SETGID", "CAP_SETUID",
	"CAP_SETPCAP", "CAP_LINUX_IMMUTABLE", "CAP_NET_BIND_SERVICE", "CAP_NET_BROADCAST",
	"CAP_NET_ADMIN", "CAP_NET_RAW", "CAP_IPC_LOCK", "CAP_IPC_OWNER",
	"CAP_SYS_MODULE", "CAP_SYS_RAWIO", "CAP_SYS_CHROOT", "CAP_SYS_PTRACE",
	"CAP_SYS_PACCT", "CAP_SYS_ADMIN", "CAP_SYS_BOOT", "CAP_SYS_NICE",
	"CAP_SYS_RESOURCE", "CAP_SYS_TIME", "CAP_SYS_TTY_CONFIG", "CAP_MKNOD",
	"CAP_LEASE", "CAP_AUDIT_WRITE", "CAP_AUDIT_CONTROL", "CAP_SETFCAP",
	"CAP_MAC_OVERRIDE", "CAP_MAC_ADMIN", "CAP_SYSLOG", "CAP_WAKE_ALARM",
	"CAP_BLOCK_SUSPEND", "CAP_AUDIT_READ", "CAP_PERFMON", "CAP_BPF",
	"CAP_CHECKPOINT_RESTORE",
}

// decodeCapabilities expands a CapEff hex mask from /proc/[pid]/status into
// capability names. The mask is 64 bits of hex with no 0x prefix.
//
// The list scan carries the mask; only drill-down expands it. Fully privileged
// processes hold every bit, and 41 strings on every row of a 300-process
// inventory is payload nobody reads.
func decodeCapabilities(mask string) []string {
	mask = strings.TrimSpace(mask)
	if mask == "" {
		return nil
	}
	bits, err := strconv.ParseUint(mask, 16, 64)
	if err != nil || bits == 0 {
		return nil
	}

	var held []string
	for bit := 0; bit < 64; bit++ {
		if bits&(1<<uint(bit)) == 0 {
			continue
		}
		if bit < len(capabilityNames) {
			held = append(held, capabilityNames[bit])
			continue
		}
		held = append(held, "CAP_"+strconv.Itoa(bit))
	}
	return held
}
