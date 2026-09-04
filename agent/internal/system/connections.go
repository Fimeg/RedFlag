package system

import "time"

type ConnectionSnapshot struct {
	Connections []Connection `json:"connections"`
	Count       int          `json:"count"`
	CollectedAt time.Time    `json:"collected_at"`
}

type Connection struct {
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
	Protocol   string `json:"protocol"`
	Family     string `json:"family"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	RemotePort int    `json:"remote_port,omitempty"`
	State      string `json:"state"`
	Inode      uint64 `json:"inode"`
}

func GetConnectionsSnapshot() (*ConnectionSnapshot, error) {
	return getConnectionsSnapshot()
}
