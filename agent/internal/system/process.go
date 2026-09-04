package system

// TopProcess represents a single process snapshot for the dashboard's
// "Top Processes" table. Fields map directly to the UI columns:
// Name, PID, CPU%, Mem%.
type TopProcess struct {
	Name string  `json:"name"`
	PID  int     `json:"pid"`
	CPU  float64 `json:"cpu"` // percent of total CPU time
	Mem  float64 `json:"mem"` // percent of total physical memory
}

// GetTopProcesses returns the top N processes sorted by CPU usage descending.
// Platform-specific implementations live in process_linux.go, process_windows.go,
// process_darwin.go.
//
// This is a snapshot primitive — no sampling interval, no background goroutine.
// The caller (loop.go) decides when to invoke it and at what cadence.
func GetTopProcesses(limit int) ([]TopProcess, error) {
	return getTopProcesses(limit)
}
