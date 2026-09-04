//go:build windows
// +build windows

package system

import (
	"encoding/csv"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// getTopProcesses on Windows uses `tasklist /FO CSV /NH` for a no-dependency
// snapshot. WMI would be more precise but requires PowerShell overhead.
func getTopProcesses(limit int) ([]TopProcess, error) {
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(string(out)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var procs []TopProcess
	for _, row := range records {
		if len(row) < 5 {
			continue
		}
		name := strings.Trim(row[0], "\"")
		pid, _ := strconv.Atoi(strings.Trim(row[1], "\""))
		// tasklist doesn't give CPU%; mem is in KB
		memStr := strings.ReplaceAll(strings.Trim(row[4], "\""), ".", "")
		memKB, _ := strconv.ParseUint(memStr, 10, 64)
		memPercent := float64(memKB) / (1024 * 1024) * 100 // rough; real impl needs total mem

		procs = append(procs, TopProcess{
			Name: name,
			PID:  pid,
			CPU:  0, // tasklist doesn't report CPU%
			Mem:  memPercent,
		})
	}

	sort.Slice(procs, func(i, j int) bool {
		return procs[i].Mem > procs[j].Mem // sort by mem since CPU unavailable
	})

	if limit > 0 && len(procs) > limit {
		procs = procs[:limit]
	}
	return procs, nil
}
