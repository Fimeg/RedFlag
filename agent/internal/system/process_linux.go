//go:build linux
// +build linux

package system

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// getTopProcesses reads /proc directly — no subprocess spawn, no cgo.
// It samples utime+stime from /proc/[pid]/stat and VmRSS from /proc/[pid]/status,
// then computes CPU percent against the total CPU ticks from /proc/stat.
func getTopProcesses(limit int) ([]TopProcess, error) {
	totalCPU, err := readTotalCPUTicks()
	if err != nil {
		return nil, fmt.Errorf("read total cpu ticks: %w", err)
	}
	if totalCPU == 0 {
		return nil, fmt.Errorf("total CPU ticks is zero")
	}

	memTotal, err := readMemTotal()
	if err != nil {
		return nil, fmt.Errorf("read mem total: %w", err)
	}
	if memTotal == 0 {
		return nil, fmt.Errorf("mem total is zero")
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	var procs []TopProcess
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		proc, err := readProcStat(pid)
		if err != nil {
			continue // process vanished or permission denied
		}

		// CPU percent = (utime + stime) / total_cpu_ticks * 100
		cpuTicks := float64(proc.utime + proc.stime)
		proc.cpu = (cpuTicks / float64(totalCPU)) * 100.0

		// Memory percent = VmRSS / MemTotal * 100
		if memTotal > 0 {
			proc.mem = (float64(proc.rss) / float64(memTotal)) * 100.0
		}

		procs = append(procs, TopProcess{
			Name: proc.name,
			PID:  pid,
			CPU:  proc.cpu,
			Mem:  proc.mem,
		})
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CPU > procs[j].CPU
	})

	if limit > 0 && len(procs) > limit {
		procs = procs[:limit]
	}
	return procs, nil
}

// procSnapshot holds raw values parsed from /proc/[pid]/stat and /proc/[pid]/status.
type procSnapshot struct {
	name  string
	utime uint64
	stime uint64
	rss   uint64 // in pages; converted to KB below
	cpu   float64
	mem   float64
}

// readProcStat parses /proc/[pid]/stat for utime (field 14) and stime (field 15),
// then reads /proc/[pid]/status for VmRSS. The comm field (field 2) is between
// the first '(' and the last ')'.
func readProcStat(pid int) (*procSnapshot, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}

	snap := &procSnapshot{}

	// Parse comm: everything between first '(' and last ')'
	firstParen := bytes.IndexByte(data, '(')
	lastParen := bytes.LastIndexByte(data, ')')
	if firstParen < 0 || lastParen <= firstParen {
		return nil, fmt.Errorf("malformed stat")
	}
	snap.name = string(data[firstParen+1 : lastParen])

	// Fields after ')' — space-separated, field 3 onward (0-indexed after the name)
	afterName := bytes.TrimSpace(data[lastParen+1:])
	fields := strings.Fields(string(afterName))
	// fields[0] = state (field 3), fields[11] = utime (field 14), fields[12] = stime (field 15)
	if len(fields) < 13 {
		return nil, fmt.Errorf("not enough fields in stat")
	}

	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return nil, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return nil, err
	}
	snap.utime = utime
	snap.stime = stime

	// Read VmRSS from /proc/[pid]/status
	statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err == nil {
		for _, line := range bytes.Split(statusData, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("VmRSS:")) {
				parts := strings.Fields(string(line))
				if len(parts) >= 2 {
					if rss, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
						snap.rss = rss // in KB
					}
				}
				break
			}
		}
	}

	return snap, nil
}

// readTotalCPUTicks reads the first "cpu " line from /proc/stat and sums all fields.
func readTotalCPUTicks() (uint64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("cpu ")) {
			fields := strings.Fields(string(line))
			var total uint64
			for _, f := range fields[1:] { // skip "cpu" label
				v, err := strconv.ParseUint(f, 10, 64)
				if err != nil {
					continue
				}
				total += v
			}
			return total, nil
		}
	}
	return 0, fmt.Errorf("no cpu line in /proc/stat")
}

// readMemTotal reads MemTotal from /proc/meminfo (in KB).
func readMemTotal() (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("MemTotal:")) {
			parts := strings.Fields(string(line))
			if len(parts) >= 2 {
				return strconv.ParseUint(parts[1], 10, 64)
			}
		}
	}
	return 0, fmt.Errorf("MemTotal not found")
}
