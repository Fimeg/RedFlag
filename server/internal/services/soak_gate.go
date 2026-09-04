package services

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// SoakGateDecision is the verdict the install-version handler uses to decide
// whether to block, warn, or pass a version selection.
type SoakGateDecision struct {
	Eligible       bool      `json:"eligible"`
	FirstScannedAt time.Time `json:"first_scanned_at"`
	SoakDays       float64   `json:"soak_days"`
	RequiredDays   float64   `json:"required_days"`
	DaysRemaining  float64   `json:"days_remaining"`
	Enforcement    string    `json:"enforcement"` // "off" | "warn" | "block"
	Unknown        bool      `json:"unknown"`
}

// SoakGateConfig reads the version soak gate configuration.
// Default: 14-day soak window, "block" enforcement.
func SoakGateConfig() (requiredDays float64, enforcement string) {
	requiredDays = 14.0
	enforcement = "block"

	if v := os.Getenv("REDFLAG_SUPPLY_CHAIN_SOAK_WINDOW_DAYS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			requiredDays = f
		}
	}
	if v := os.Getenv("REDFLAG_SUPPLY_CHAIN_SOAK_ENFORCEMENT"); v != "" {
		switch strings.ToLower(v) {
		case "off", "warn", "block":
			enforcement = strings.ToLower(v)
		}
	}
	return
}

// EvaluateSoakGate computes the soak gate decision for a single version.
func EvaluateSoakGate(firstScannedAt time.Time, requiredDays float64, enforcement string) SoakGateDecision {
	dec := SoakGateDecision{
		RequiredDays: requiredDays,
		Enforcement:  enforcement,
	}
	if firstScannedAt.IsZero() {
		dec.Unknown = true
		return dec
	}

	dec.FirstScannedAt = firstScannedAt
	dec.SoakDays = time.Since(firstScannedAt).Hours() / 24.0
	dec.DaysRemaining = requiredDays - dec.SoakDays
	if dec.DaysRemaining < 0 {
		dec.DaysRemaining = 0
	}

	if dec.SoakDays >= requiredDays {
		dec.Eligible = true
		return dec
	}

	if enforcement == "block" {
		dec.Eligible = false
	} else {
		dec.Eligible = true
	}
	return dec
}

// SoakGateWarnMessage returns a human-readable message for non-eligible decisions.
func SoakGateWarnMessage(dec SoakGateDecision) string {
	return fmt.Sprintf(
		"version has been in the fleet for %.1f days (soak threshold %.1f days, %.1f days remaining)",
		dec.SoakDays, dec.RequiredDays, dec.DaysRemaining,
	)
}
