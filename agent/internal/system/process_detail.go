package system

import "time"

// FullProcess represents a complete process snapshot with osquery-level detail.
// Fields mirror the osquery `processes` table schema plus related data for
// drill-down (open files, sockets, env, memory map, namespaces, pipes).
type FullProcess struct {
	// Core identification
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`    // /proc/[pid]/exe symlink target
	Cmdline string `json:"cmdline"`           // /proc/[pid]/cmdline (NUL→space)
	Cwd     string `json:"cwd,omitempty"`     // /proc/[pid]/cwd symlink target

	// State and identity
	State string `json:"state"` // R/S/D/Z/T from /proc/[pid]/stat
	UID   uint32 `json:"uid"`
	GID   uint32 `json:"gid"`
	EUID  uint32 `json:"euid"`
	EGID  uint32 `json:"egid"`
	User  string `json:"user"`  // resolved from /etc/passwd
	Group string `json:"group"` // resolved from /etc/group

	// Terminal
	TTY     int    `json:"tty"`
	TTYName string `json:"tty_name,omitempty"`

	// Resource usage
	CPUSecondsUser   float64 `json:"cpu_seconds_user"`   // utime / clock_ticks
	CPUSecondsSystem float64 `json:"cpu_seconds_system"` // stime / clock_ticks
	CPUPercent       float64 `json:"cpu_percent"`        // vs total CPU ticks
	RSSBytes         uint64  `json:"rss_bytes"`          // VmRSS from /proc/[pid]/status
	VMSBytes         uint64  `json:"vms_bytes"`          // VmSize from /proc/[pid]/status
	MemPercent       float64 `json:"mem_percent"`
	Threads          int     `json:"threads"` // Threads field from /proc/[pid]/status

	// Process metadata
	Nice             int    `json:"nice"`
	StartTimeSeconds uint64 `json:"start_time_seconds"` // starttime from /proc/[pid]/stat
	ParentPID        int    `json:"parent_pid"`
	ProcessGroupID   int    `json:"process_group_id"`

	// On-disk check
	OnDisk int `json:"on_disk"` // 1=yes, 0=no, -1=unknown (matches osquery)

	// Elevation
	ElevationStatus string `json:"elevation_status,omitempty"` // "elevated" if uid!=euid

	// Ownership — cgroup, systemd unit, container. Inlined into the JSON.
	ProcessOwner

	// Effective capability mask from /proc/[pid]/status. Decoded into names on
	// drill-down only — see Capabilities below.
	CapabilitiesEffective string `json:"capabilities_effective,omitempty"`

	// Disk I/O (from /proc/[pid]/io, may be empty for other users' processes)
	DiskBytesRead    uint64 `json:"disk_bytes_read,omitempty"`
	DiskBytesWritten uint64 `json:"disk_bytes_written,omitempty"`

	// Related data — populated only on drill-down (GetProcessDetail), not on list scan
	OpenFiles       []ProcessOpenFile      `json:"open_files,omitempty"`
	OpenSockets     []ProcessOpenSocket    `json:"open_sockets,omitempty"`
	OpenPipes       []ProcessOpenPipe      `json:"open_pipes,omitempty"`
	EnvironmentVars []string               `json:"environment_vars,omitempty"` // keys only
	MemoryMap       []ProcessMemoryMap     `json:"memory_map,omitempty"`
	Namespaces      []ProcessNamespace     `json:"namespaces,omitempty"`
	ListeningPorts  []ProcessListeningPort `json:"listening_ports,omitempty"`
	Capabilities    []string               `json:"capabilities,omitempty"` // effective set, decoded
}

// Related data types — stored as JSONB per relation in the database.

type ProcessOpenFile struct {
	FD   int    `json:"fd"`
	Path string `json:"path"`
	Type string `json:"type,omitempty"` // file, socket, pipe, etc.
}

type ProcessOpenSocket struct {
	FD          int    `json:"fd"`
	Family      string `json:"family"`  // IPv4, IPv6, UNIX
	Protocol    string `json:"protocol"` // TCP, UDP
	LocalAddr   string `json:"local_addr"`
	LocalPort   int    `json:"local_port"`
	RemoteAddr  string `json:"remote_addr"`
	RemotePort  int    `json:"remote_port"`
	State       string `json:"state"`
	Inode       uint64 `json:"inode"`
	Path        string `json:"path,omitempty"` // for UNIX sockets
}

type ProcessOpenPipe struct {
	FD    int    `json:"fd"`
	Inode uint64 `json:"inode"`
	Mode  string `json:"mode,omitempty"` // r/w
	Type  string `json:"type,omitempty"` // named vs anonymous
}

type ProcessMemoryMap struct {
	Start       uint64 `json:"start"`
	End         uint64 `json:"end"`
	Permissions string `json:"permissions"` // r/w/x/p
	Offset      uint64 `json:"offset"`
	Device      string `json:"device"`
	Inode       uint64 `json:"inode"`
	Path        string `json:"path,omitempty"`
}

type ProcessNamespace struct {
	Type  string `json:"type"`  // cgroup, ipc, mnt, net, pid, user, uts
	Inode string `json:"inode"` // string because kernel shows as "[4026531835]"
}

type ProcessListeningPort struct {
	Protocol  string `json:"protocol"` // TCP, UDP
	LocalAddr string `json:"local_addr"`
	LocalPort int    `json:"local_port"`
	FD        int    `json:"fd,omitempty"`
	Socket    uint64 `json:"socket,omitempty"`
}

// FullProcessSnapshot is the complete result of an on-demand process scan.
type FullProcessSnapshot struct {
	Processes    []FullProcess `json:"processes"`
	ProcessCount int           `json:"process_count"`
	ScannedAt    time.Time     `json:"scanned_at"`
	DurationMs   int64         `json:"duration_ms"`
}

// GetFullProcessSnapshot reads /proc to build a complete process inventory.
// Platform-specific implementations live in process_detail_linux.go and
// process_detail_other.go (stub returning "not supported").
func GetFullProcessSnapshot() (*FullProcessSnapshot, error) {
	return getFullProcessSnapshot()
}

// ProcessCaps holds per-process data collection limits.
// Zero values mean "no cap" (but the underlying /proc data is still bounded by the kernel).
type ProcessCaps struct {
	MaxOpenFiles      int `json:"max_open_files"`
	MaxSockets        int `json:"max_sockets"`
	MaxPipes          int `json:"max_pipes"`
	MaxMemoryMap      int `json:"max_memory_map"`
	MaxNamespaces     int `json:"max_namespaces"`
	MaxEnvKeys        int `json:"max_env_keys"`
	MaxListeningPorts int `json:"max_listening_ports"`
}

// GetProcessDetail reads the related data (open files, sockets, env, etc.) for a
// specific process. Called on drill-down, not on the initial list scan.
func GetProcessDetail(pid int, caps ProcessCaps) (*FullProcess, error) {
	return getProcessDetail(pid, caps)
}
