package models

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// ProcessSnapshot represents one on-demand process scan for an agent.
type ProcessSnapshot struct {
	ID             uuid.UUID `json:"id" db:"id"`
	AgentID        uuid.UUID `json:"agent_id" db:"agent_id"`
	CommandID      string    `json:"command_id" db:"command_id"`
	ProcessCount   int       `json:"process_count" db:"process_count"`
	ScannedAt      time.Time `json:"scanned_at" db:"scanned_at"`
	ScanDurationMs int       `json:"scan_duration_ms" db:"scan_duration_ms"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// Process represents a single process within a snapshot.
type Process struct {
	ID                uuid.UUID `json:"id" db:"id"`
	SnapshotID        uuid.UUID `json:"snapshot_id" db:"snapshot_id"`
	AgentID           uuid.UUID `json:"agent_id" db:"agent_id"`
	PID               int       `json:"pid" db:"pid"`
	Name              string    `json:"name" db:"name"`
	Path              string    `json:"path" db:"path"`
	Cmdline           string    `json:"cmdline" db:"cmdline"`
	Cwd               string    `json:"cwd" db:"cwd"`
	State             string    `json:"state" db:"state"`
	UID               int       `json:"uid" db:"uid"`
	GID               int       `json:"gid" db:"gid"`
	EUID              int       `json:"euid" db:"euid"`
	EGID              int       `json:"egid" db:"egid"`
	User              string    `json:"user" db:"user"`
	Group             string    `json:"group" db:"group"`
	TTY               int       `json:"tty" db:"tty"`
	TTYName           string    `json:"tty_name" db:"tty_name"`
	CPUSecondsUser    float64   `json:"cpu_seconds_user" db:"cpu_seconds_user"`
	CPUSecondsSystem  float64   `json:"cpu_seconds_system" db:"cpu_seconds_system"`
	CPUPercent        float64   `json:"cpu_percent" db:"cpu_percent"`
	RSSBytes          int64     `json:"rss_bytes" db:"rss_bytes"`
	VMSBytes          int64     `json:"vms_bytes" db:"vms_bytes"`
	MemPercent        float64   `json:"mem_percent" db:"mem_percent"`
	Threads           int       `json:"threads" db:"threads"`
	Nice              int       `json:"nice" db:"nice"`
	StartTimeSeconds  int64     `json:"start_time_seconds" db:"start_time_seconds"`
	ParentPID         int       `json:"parent_pid" db:"parent_pid"`
	ProcessGroupID    int       `json:"process_group_id" db:"process_group_id"`
	ElevationStatus   string    `json:"elevation_status" db:"elevation_status"`
	OnDisk            int       `json:"on_disk" db:"on_disk"`
	DiskBytesRead     int64     `json:"disk_bytes_read" db:"disk_bytes_read"`
	DiskBytesWritten  int64     `json:"disk_bytes_written" db:"disk_bytes_written"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

// ProcessRelated represents related data (files, sockets, etc.) for a process.
type ProcessRelated struct {
	ID           uuid.UUID `json:"id" db:"id"`
	ProcessID    uuid.UUID `json:"process_id" db:"process_id"`
	RelationType string    `json:"relation_type" db:"relation_type"`
	Data         JSONB     `json:"data" db:"data"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// ProcessScanRequest is the payload from the agent's process scan report.
type ProcessScanRequest struct {
	AgentID   uuid.UUID          `json:"agent_id"`
	CommandID string             `json:"command_id"`
	Timestamp time.Time          `json:"timestamp"`
	Snapshot  ProcessSnapshotData `json:"snapshot"`
}

// ProcessSnapshotData mirrors the agent's FullProcessSnapshot for deserialization.
type ProcessSnapshotData struct {
	Processes    []ProcessData `json:"processes"`
	ProcessCount int           `json:"process_count"`
	ScannedAt    time.Time     `json:"scanned_at"`
	DurationMs   int64         `json:"duration_ms"`
}

// ProcessData mirrors a single FullProcess from the agent.
type ProcessData struct {
	PID               int                     `json:"pid"`
	Name              string                  `json:"name"`
	Path              string                  `json:"path,omitempty"`
	Cmdline           string                  `json:"cmdline"`
	Cwd               string                  `json:"cwd,omitempty"`
	State             string                  `json:"state"`
	UID               uint32                  `json:"uid"`
	GID               uint32                  `json:"gid"`
	EUID              uint32                  `json:"euid"`
	EGID              uint32                  `json:"egid"`
	User              string                  `json:"user"`
	Group             string                  `json:"group"`
	TTY               int                     `json:"tty"`
	TTYName           string                  `json:"tty_name,omitempty"`
	CPUSecondsUser    float64                 `json:"cpu_seconds_user"`
	CPUSecondsSystem  float64                 `json:"cpu_seconds_system"`
	CPUPercent        float64                 `json:"cpu_percent"`
	RSSBytes          uint64                  `json:"rss_bytes"`
	VMSBytes          uint64                  `json:"vms_bytes"`
	MemPercent        float64                 `json:"mem_percent"`
	Threads           int                     `json:"threads"`
	Nice              int                     `json:"nice"`
	StartTimeSeconds  uint64                  `json:"start_time_seconds"`
	ParentPID         int                     `json:"parent_pid"`
	ProcessGroupID    int                     `json:"process_group_id"`
	OnDisk            int                     `json:"on_disk"`
	ElevationStatus   string                  `json:"elevation_status,omitempty"`
	DiskBytesRead     uint64                  `json:"disk_bytes_read,omitempty"`
	DiskBytesWritten  uint64                  `json:"disk_bytes_written,omitempty"`
	OpenFiles         []ProcessOpenFileData   `json:"open_files,omitempty"`
	OpenSockets       []ProcessOpenSocketData `json:"open_sockets,omitempty"`
	OpenPipes         []ProcessOpenPipeData   `json:"open_pipes,omitempty"`
	EnvironmentVars   []string                `json:"environment_vars,omitempty"`
	MemoryMap         []ProcessMemoryMapData  `json:"memory_map,omitempty"`
	Namespaces        []ProcessNamespaceData  `json:"namespaces,omitempty"`
	ListeningPorts    []ProcessListeningPortData `json:"listening_ports,omitempty"`
}

// Related data types for deserialization.

type ProcessOpenFileData struct {
	FD   int    `json:"fd"`
	Path string `json:"path"`
	Type string `json:"type,omitempty"`
}

type ProcessOpenSocketData struct {
	FD         int    `json:"fd"`
	Family     string `json:"family"`
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr"`
	RemotePort int    `json:"remote_port"`
	State      string `json:"state"`
	Inode      uint64 `json:"inode"`
	Path       string `json:"path,omitempty"`
}

type ProcessOpenPipeData struct {
	FD    int    `json:"fd"`
	Inode uint64 `json:"inode"`
	Mode  string `json:"mode,omitempty"`
	Type  string `json:"type,omitempty"`
}

type ProcessMemoryMapData struct {
	Start       uint64 `json:"start"`
	End         uint64 `json:"end"`
	Permissions string `json:"permissions"`
	Offset      uint64 `json:"offset"`
	Device      string `json:"device"`
	Inode       uint64 `json:"inode"`
	Path        string `json:"path,omitempty"`
}

type ProcessNamespaceData struct {
	Type  string `json:"type"`
	Inode string `json:"inode"`
}

type ProcessListeningPortData struct {
	Protocol  string `json:"protocol"`
	LocalAddr string `json:"local_addr"`
	LocalPort int    `json:"local_port"`
	FD        int    `json:"fd,omitempty"`
	Socket    uint64 `json:"socket,omitempty"`
}

// ProcessSnapshotResponse is the API response for the latest snapshot.
type ProcessSnapshotResponse struct {
	Snapshot  *ProcessSnapshot `json:"snapshot"`
	Processes []Process        `json:"processes"`
}

// ProcessDetailResponse is the API response for a single process with related data.
type ProcessDetailResponse struct {
	Process        Process          `json:"process"`
	OpenFiles      []ProcessRelated `json:"open_files,omitempty"`
	OpenSockets    []ProcessRelated `json:"open_sockets,omitempty"`
	OpenPipes      []ProcessRelated `json:"open_pipes,omitempty"`
	Environment    []ProcessRelated `json:"environment,omitempty"`
	MemoryMap      []ProcessRelated `json:"memory_map,omitempty"`
	Namespaces     []ProcessRelated `json:"namespaces,omitempty"`
	ListeningPorts []ProcessRelated `json:"listening_ports,omitempty"`
}
