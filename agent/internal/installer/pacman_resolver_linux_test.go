//go:build linux

package installer

import "testing"

func TestParsePacmanPrint(t *testing.T) {
	repositories, err := parsePacmanPrint("linux|1:6.19.14-1|core\nmkinitcpio|39.2-3|core\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := repositories["linux@1:6.19.14-1"]; got != "core" {
		t.Fatalf("epoch-bearing identity repository = %q, want core", got)
	}
	if got := repositories["mkinitcpio@39.2-3"]; got != "core" {
		t.Fatalf("dependency repository = %q, want core", got)
	}
}

func TestParsePacmanPrintRefusesMalformedIdentity(t *testing.T) {
	for _, output := range []string{"", "linux 6.19 core", "linux|6.19|"} {
		if _, err := parsePacmanPrint(output); err == nil {
			t.Fatalf("parsePacmanPrint(%q) succeeded", output)
		}
	}
}
