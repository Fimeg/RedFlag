//go:build darwin
// +build darwin

package system

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// getTopProcesses on macOS uses `ps aux` — no /proc, no sysctl for per-process.
// Can be optimized later with sysctl KERN_PROC if needed.
func getTopProcesses(limit int) ([]TopProcess, error) {
	out, err := exec.Command("ps", "aux", "--sort=-%cpu").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	var procs []TopProcess
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		name := fields[10]

		procs = append(procs, TopProcess{
			Name: name,
			PID:  pid,
			CPU:  cpu,
			Mem:  mem,
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
