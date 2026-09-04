package services

import (
	_ "embed"
	"encoding/json"
	"log"
)

// Supply-chain posture — RedFlag's attestation of its OWN dependency hygiene,
// folded into the signed release manifest so it rides the same Ed25519 trust
// root as the binaries. The installer verifies the manifest signature before
// trusting any of this, so the posture cannot be forged by editing a file next
// to the binary: tamper breaks the signature and the install refuses.
//
// The content is generated at build time by scripts/dep-scan.sh (the same gate
// that fails the release on an un-accepted reachable vulnerability) and embedded
// here. A bare `go build` outside that pipeline embeds the committed stub with
// Attested=false — an honest "this build was not gated", which the installer
// treats as a refusal-worthy posture.

//go:embed posture-build.json
var postureBuildJSON []byte

// DependencyException is a known, accepted reachable vulnerability with the
// documented reason it is tolerated. Mirrors a line in .govulncheck-allow.
type DependencyException struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// EcosystemScan is one scanner's verdict over one ecosystem.
type EcosystemScan struct {
	Ecosystem string `json:"ecosystem"` // go, npm, cargo
	Tool      string `json:"tool"`      // govulncheck, npm-audit, cargo-audit
	Status    string `json:"status"`    // clean, accepted, blocked
	Accepted  int    `json:"accepted"`
	Blocked   int    `json:"blocked"`
}

// SupplyChainPosture is RedFlag's own dependency attestation for this build.
type SupplyChainPosture struct {
	// Attested is true only when this posture was produced by the gated build
	// pipeline. A stub/dev build is Attested=false.
	Attested    bool                  `json:"attested"`
	GeneratedAt int64                 `json:"generated_at"`
	Substrate   map[string]string     `json:"substrate"` // go, rustc, node, npm, docker versions
	Scans       []EcosystemScan       `json:"scans"`
	Exceptions  []DependencyException  `json:"exceptions"`
}

// Blocked reports whether any ecosystem carries an un-accepted reachable vuln.
// A clean build never embeds a blocked scan (the gate fails the release first),
// so this is defense-in-depth for a tampered or hand-built artifact.
func (p SupplyChainPosture) Blocked() bool {
	for _, s := range p.Scans {
		if s.Blocked > 0 {
			return true
		}
	}
	return false
}

// loadEmbeddedPosture parses the build-time posture. A parse failure is logged
// and degraded to an explicit un-attested posture rather than a crash — the
// manifest still signs, and the installer refuses on Attested=false.
func loadEmbeddedPosture() SupplyChainPosture {
	var p SupplyChainPosture
	if err := json.Unmarshal(postureBuildJSON, &p); err != nil {
		log.Printf("[ERROR] [server] [posture] embedded posture-build.json unparseable: %v", err)
		return SupplyChainPosture{Attested: false}
	}
	return p
}

// EmbeddedPosture returns the build-time supply-chain posture for this server.
func EmbeddedPosture() SupplyChainPosture {
	return loadEmbeddedPosture()
}
