package services

import (
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/config"
)

type TimezoneService struct {
	config *config.Config
}

func NewTimezoneService(config *config.Config) *TimezoneService {
	return &TimezoneService{
		config: config,
	}
}

// GetTimezoneLocation returns the configured timezone as a time.Location
func (s *TimezoneService) GetTimezoneLocation() (*time.Location, error) {
	return time.LoadLocation(s.config.Timezone)
}

// FormatTimeForTimezone formats a time.Time according to the configured timezone
func (s *TimezoneService) FormatTimeForTimezone(t time.Time) (time.Time, error) {
	loc, err := s.GetTimezoneLocation()
	if err != nil {
		return t, err
	}
	return t.In(loc), nil
}

// GetNowInTimezone returns the current time in the configured timezone
func (s *TimezoneService) GetNowInTimezone() (time.Time, error) {
	return s.FormatTimeForTimezone(time.Now())
}

// GetAvailableTimezones returns a list of common timezones
func (s *TimezoneService) GetAvailableTimezones() []TimezoneOption {
	return []TimezoneOption{
		{Value: "UTC", Label: "UTC (Coordinated Universal Time)"},
		{Value: "America/New_York", Label: "Eastern Time (ET)"},
		{Value: "America/Chicago", Label: "Central Time (CT)"},
		{Value: "America/Denver", Label: "Mountain Time (MT)"},
		{Value: "America/Los_Angeles", Label: "Pacific Time (PT)"},
		{Value: "Europe/London", Label: "London (GMT)"},
		{Value: "Europe/Paris", Label: "Paris (CET)"},
		{Value: "Europe/Berlin", Label: "Berlin (CET)"},
		{Value: "Asia/Tokyo", Label: "Tokyo (JST)"},
		{Value: "Asia/Shanghai", Label: "Shanghai (CST)"},
		{Value: "Australia/Sydney", Label: "Sydney (AEDT)"},
	}
}

type TimezoneOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}