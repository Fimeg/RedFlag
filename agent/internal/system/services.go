package system

import "time"

type ServiceSnapshot struct {
	Manager     string    `json:"manager"`
	Services    []Service `json:"services"`
	Count       int       `json:"count"`
	Running     int       `json:"running"`
	Failed      int       `json:"failed"`
	CollectedAt time.Time `json:"collected_at"`
}

type Service struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	LoadState   string `json:"load_state,omitempty"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state,omitempty"`
}

func GetServicesSnapshot() (*ServiceSnapshot, error) { return getServicesSnapshot() }
