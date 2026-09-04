package system

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type windowsCPUInfoJSON struct {
	Name                      string          `json:"Name"`
	NumberOfCores             json.RawMessage `json:"NumberOfCores"`
	NumberOfLogicalProcessors json.RawMessage `json:"NumberOfLogicalProcessors"`
}

func parseWindowsCPUInfoJSON(data []byte) (*CPUInfo, error) {
	var item windowsCPUInfoJSON
	if err := json.Unmarshal(data, &item); err != nil {
		var items []windowsCPUInfoJSON
		if arrayErr := json.Unmarshal(data, &items); arrayErr != nil || len(items) == 0 {
			return nil, fmt.Errorf("invalid Windows CPU JSON: %w", err)
		}
		item = items[0]
	}

	cpu := &CPUInfo{
		ModelName: strings.TrimSpace(item.Name),
		Cores:     parseWindowsJSONInt(item.NumberOfCores),
		Threads:   parseWindowsJSONInt(item.NumberOfLogicalProcessors),
	}
	if cpu.Threads == 0 {
		cpu.Threads = cpu.Cores
	}
	if cpu.ModelName == "" && cpu.Cores == 0 && cpu.Threads == 0 {
		return nil, fmt.Errorf("Windows CPU JSON did not include usable fields")
	}
	return cpu, nil
}

func parseWindowsJSONInt(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}

	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}

	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f)
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return n
		}
	}

	return 0
}
