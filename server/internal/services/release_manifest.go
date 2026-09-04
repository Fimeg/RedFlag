package services

// Release manifest — the cold-start trust root and component healthcheck anchor.
//
// Two audiences, one document:
// 1. Install-time: the installer fetches the manifest, verifies the Ed25519
//    signature over the verbatim body, then checks every downloaded binary
//    against its matching artifacts entry before executing anything.
// 2. Post-install healthcheck: walks Components, runs version_cmd on each
//    installed binary, verifies provisioning state, emits a signed checkoff
//    report (local journal + server security event).
//
// Signed with the server's instance Ed25519 key (TOFU pubkey on first contact,
// pinned on upgrade). The signed bytes are the verbatim JSON; the signature
// travels in X-Content-Signature, the key fingerprint in X-Key-Id.
//
// CI generates the authoritative manifest at release time (embedded in the
// server binary); at serve time the server re-signs with its instance key.
// A release whose manifest lists a component without a built, version-self-
// reporting artifact fails the release gate — no silent drop-out.

// ManifestArtifact is one released binary's expected identity.
type ManifestArtifact struct {
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	Filename     string `json:"filename"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
}

// ManifestComponent describes one installable/checkable piece of the release.
// Required components block install completion; optional ones are skipped
// gracefully when unavailable.
type ManifestComponent struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`                   // docker, binary, embedded
	Required     bool     `json:"required"`
	VersionCmd   string   `json:"version_cmd"`            // e.g. "--version"
	Provisioning []string `json:"provisioning,omitempty"` // healthcheck steps (desktop: autostart_entry, redflag-local_group, desktop_user_membership)
}

// ReleaseManifest is the signed component+artifact catalog for one version.
//
// SupplyChain is the running server's own dependency attestation (built-time
// scan verdict + accepted exceptions + build substrate). It rides the manifest
// signature so the installer can show "safe checks" without a second trust root,
// and refuse on an un-attested or blocked posture.
type ReleaseManifest struct {
	Version     string              `json:"version"`
	GeneratedAt int64               `json:"generated_at"`
	KeyID       string              `json:"key_id"`
	Components  []ManifestComponent `json:"components"`
	Artifacts   []ManifestArtifact  `json:"artifacts"`
	SupplyChain SupplyChainPosture  `json:"supply_chain"`
}

// ComponentCatalog returns the fixed component set for a release.
// This is the authoritative list — CI's gate job enforces that every component
// here has a built, version-self-reporting artifact.
func ComponentCatalog() []ManifestComponent {
	return []ManifestComponent{
		{Name: "server", Kind: "binary", Required: true, VersionCmd: "--version"},
		{Name: "agent", Kind: "binary", Required: true, VersionCmd: "--version"},
		{Name: "helper", Kind: "binary", Required: true, VersionCmd: "--version"},
		{Name: "desktop", Kind: "binary", Required: false, VersionCmd: "--version",
			Provisioning: []string{"autostart_entry", "redflag-local_group", "desktop_user_membership"}},
		{Name: "web", Kind: "embedded", Required: true},
		// installer (INSTALL-002): release-gate presence check only, not a
		// post-install healthcheck target — it's a delivery mechanism, not a
		// running artifact (the thing it installs, "server", is already its
		// own entry above). No VersionCmd: an MSI can't self-report a
		// version by being executed. Windows-only today (RedFlagSetup.msi),
		// so Required: false until macOS/Linux installers exist too.
		{Name: "installer", Kind: "binary", Required: false},
	}
}
