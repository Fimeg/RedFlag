package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/gofrs/uuid/v5"
)

func TestGatedLocalApprovalEcosystems(t *testing.T) {
	for _, packageType := range []string{"apt", "dnf", "pacman"} {
		if !gatedLocalApproval(packageType) {
			t.Errorf("%s is not routed through local authority", packageType)
		}
	}
	for _, packageType := range []string{"docker", "winget", "windows_update", "cargo"} {
		if gatedLocalApproval(packageType) {
			t.Errorf("%s entered local authority without a migrated backend", packageType)
		}
	}
}

func TestLocalApprovalRefusesFleetModeBeforeResolution(t *testing.T) {
	id, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	_, err = HandleLocalApprove(context.Background(), &config.Config{AgentID: id, Token: "fleet"}, LocalApproveRequest{
		PackageType: "pacman", PackageName: "linux", Operator: "operator", OverrideReason: "accepted",
	})
	if !errors.Is(err, ErrApprovalFleetMode) {
		t.Fatalf("fleet approval error = %v", err)
	}
}

func TestPacmanUnsupportedOSVRequiresReasonBeforeResolution(t *testing.T) {
	id, err := uuid.NewV4()
	if err != nil {
		t.Fatal(err)
	}
	_, err = HandleLocalApprove(context.Background(), &config.Config{AgentID: id}, LocalApproveRequest{
		PackageType: "pacman", PackageName: "not-a-real-package", Operator: "operator",
	})
	if !errors.Is(err, ErrApprovalBlocked) {
		t.Fatalf("pacman approval error = %v", err)
	}
}
