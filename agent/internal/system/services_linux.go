//go:build linux

package system

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type systemdUnit struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

func getServicesSnapshot() (*ServiceSnapshot, error) {
	command := exec.Command("systemctl", "list-units", "--type=service", "--all", "--output=json", "--no-pager")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list systemd services: %w", err)
	}
	var units []systemdUnit
	if err := json.Unmarshal(output, &units); err != nil {
		return nil, fmt.Errorf("decode systemd service list: %w", err)
	}
	snapshot := &ServiceSnapshot{Manager: "systemd", CollectedAt: time.Now().UTC()}
	for _, unit := range units {
		name := strings.TrimSuffix(unit.Unit, ".service")
		snapshot.Services = append(snapshot.Services, Service{Name: name, Description: unit.Description, LoadState: unit.Load, ActiveState: unit.Active, SubState: unit.Sub})
		if unit.Active == "active" {
			snapshot.Running++
		}
		if unit.Active == "failed" {
			snapshot.Failed++
		}
	}
	sort.Slice(snapshot.Services, func(i, j int) bool {
		if snapshot.Services[i].ActiveState != snapshot.Services[j].ActiveState {
			return snapshot.Services[i].ActiveState == "failed"
		}
		return snapshot.Services[i].Name < snapshot.Services[j].Name
	})
	snapshot.Count = len(snapshot.Services)
	return snapshot, nil
}
