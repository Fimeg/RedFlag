package config

import (
	"testing"

	"github.com/gofrs/uuid/v5"
)

func TestInitializeStandaloneIsStable(t *testing.T) {
	cfg := &Config{}
	if err := cfg.InitializeStandalone(); err != nil {
		t.Fatal(err)
	}
	first := cfg.AgentID
	if first == uuid.Nil || !cfg.IsStandalone() || cfg.IsRegistered() {
		t.Fatalf("standalone identity not established: id=%s", first)
	}
	if err := cfg.InitializeStandalone(); err != nil {
		t.Fatal(err)
	}
	if cfg.AgentID != first {
		t.Fatalf("standalone identity changed: %s -> %s", first, cfg.AgentID)
	}
}

func TestInitializeStandaloneRefusesFleetMaterial(t *testing.T) {
	for name, cfg := range map[string]*Config{
		"registration token": {RegistrationToken: "register"},
		"access token":       {Token: "access"},
		"refresh token":      {RefreshToken: "refresh"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := cfg.InitializeStandalone(); err == nil {
				t.Fatal("fleet material became standalone authority")
			}
		})
	}
}

func TestPartialFleetEnrollmentIsNotStandalone(t *testing.T) {
	id, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{AgentID: id, RefreshToken: "refresh"}
	if cfg.IsStandalone() {
		t.Fatal("partial fleet enrollment reported as standalone")
	}
}
