package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ResolvedArtifact is one package resolved to the exact artifact a pinned install
// would fetch, with the SHA256 the package manager verifies against its GPG-signed
// repository metadata. It is the unit the server pins and the capability token binds.
type ResolvedArtifact struct {
	Name    string // package name as the package manager records it
	Version string // exact version (dnf: EVR; apt: Version field)
	SHA256  string // lowercase hex; canonical artifact hash from signed metadata
}

// packageNamePattern bounds package identifiers to the characters real dnf/apt
// names use. Resolution shells out (no shell, argv only — so this is defense in
// depth, not the sole guard) and the name originates in a server command, so we
// refuse anything that isn't a plausible package reference rather than pass it on.
var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+:~-]*$`)

func validatePackageName(name string) error {
	if name == "" {
		return fmt.Errorf("empty package name")
	}
	if len(name) > 256 {
		return fmt.Errorf("package name too long")
	}
	if !packageNamePattern.MatchString(name) {
		return fmt.Errorf("package name contains disallowed characters: %q", name)
	}
	return nil
}

// ResolveArtifactSHA256 resolves the canonical artifact hash for a package on the
// given package manager. When version is non-empty, resolution is for that exact
// target version, not the current candidate. It is the agent-side hash source for
// the registry: the server cannot reach an agent's repos, so the agent reads the
// hash its own signed metadata anchors. Returns an error (never a wrong hash)
// when resolution is not possible for the package type or the package cannot be
// resolved.
func ResolveArtifactSHA256(packageType, packageName, version string) (*ResolvedArtifact, error) {
	if err := validatePackageName(packageName); err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_invalid_name type=%s error=%w", packageType, err)
	}
	if version != "" {
		if err := validatePackageName(version); err != nil {
			return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_invalid_version type=%s error=%w", packageType, err)
		}
	}

	switch packageType {
	case "dnf":
		return resolveDNFArtifact(packageName, version)
	case "apt":
		return resolveAPTArtifact(packageName, version)
	default:
		return nil, fmt.Errorf("artifact hash resolution not supported for package type %q", packageType)
	}
}

// resolveDNFArtifact downloads the package to a private temp dir with `dnf
// download` (which verifies the artifact against the signed primary.xml during
// download), reads its exact NEVRA via rpm, and hashes the file. The download is
// unprivileged and the temp dir is removed afterward.
func resolveDNFArtifact(packageName, version string) (*ResolvedArtifact, error) {
	tmpDir, err := os.MkdirTemp("", "redflag-hash-")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_tmpdir error=%w", err)
	}
	defer os.RemoveAll(tmpDir)

	target := packageName
	if version != "" {
		target = packageName + "-" + version
	}
	out, err := exec.Command("dnf", "download", "--destdir", tmpDir, target).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_dnf_download pkg=%s error=%w output=%s",
			target, err, strings.TrimSpace(string(out)))
	}

	rpmPath, err := singleRPMInDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_dnf_artifact pkg=%s error=%w", packageName, err)
	}

	sum, err := fileSHA256(rpmPath)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_dnf_hash pkg=%s error=%w", packageName, err)
	}

	name, version := rpmNameVersion(rpmPath, packageName)
	return &ResolvedArtifact{Name: name, Version: version, SHA256: sum}, nil
}

// singleRPMInDir returns the path of the one binary .rpm a `dnf download` of a
// single named package produces. Source rpms are dropped first: dnf5 pulls the
// matching .src.rpm alongside the binary when the source lives in an enabled repo
// (COPR repos commonly do), but the source is never an install artifact, so it
// must not count toward ambiguity. More than one *binary* rpm remaining means the
// request was genuinely ambiguous and we refuse rather than guess the pinned one.
func singleRPMInDir(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.rpm"))
	if err != nil {
		return "", err
	}
	binaries := matches[:0]
	for _, m := range matches {
		if strings.HasSuffix(m, ".src.rpm") {
			continue
		}
		binaries = append(binaries, m)
	}
	switch len(binaries) {
	case 0:
		return "", fmt.Errorf("no binary rpm downloaded")
	case 1:
		return binaries[0], nil
	default:
		return "", fmt.Errorf("ambiguous download: %d binary rpms produced", len(binaries))
	}
}

// rpmNameVersion reads the exact NAME and EVR from a downloaded rpm header.
// It formats EVR the same way dnf check-update does: epoch is included only when
// non-zero. Falls back to the requested name and an empty version if rpm is
// unavailable — the hash is still authoritative, only the recorded version label
// degrades.
func rpmNameVersion(rpmPath, requestedName string) (name, version string) {
	out, err := exec.Command("rpm", "-qp", "--nosignature", "--queryformat", "%{NAME}|%{EPOCHNUM}|%{VERSION}|%{RELEASE}", rpmPath).Output()
	if err != nil {
		return requestedName, ""
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) != 4 || parts[0] == "" {
		return requestedName, ""
	}
	return parts[0], formatRPMVersion(parts[1], parts[2], parts[3])
}

func formatRPMVersion(epoch, version, release string) string {
	epoch = strings.TrimSpace(epoch)
	version = strings.TrimSpace(version)
	release = strings.TrimSpace(release)
	if version == "" {
		return ""
	}

	evr := version
	if release != "" {
		evr += "-" + release
	}
	if epoch != "" && epoch != "0" && epoch != "(none)" {
		evr = epoch + ":" + evr
	}
	return evr
}

// resolveAPTArtifact reads the target version's .deb SHA256 straight from the
// signed apt index via apt-cache. With no explicit version, it uses the current
// candidate. No download is needed: apt already exposes the hash the
// Release/Packages index commits to.
func resolveAPTArtifact(packageName, version string) (*ResolvedArtifact, error) {
	candidate := version
	if candidate == "" {
		var err error
		candidate, err = aptCandidateVersion(packageName)
		if err != nil {
			return nil, err
		}
	}

	out, err := exec.Command("apt-cache", "show", packageName+"="+candidate).Output()
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_apt_show pkg=%s version=%s error=%w",
			packageName, candidate, err)
	}

	sum := aptFieldFromStanza(string(out), "SHA256:")
	if sum == "" {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_apt_no_sha256 pkg=%s version=%s", packageName, candidate)
	}
	return &ResolvedArtifact{Name: packageName, Version: candidate, SHA256: strings.ToLower(sum)}, nil
}

// aptCandidateVersion returns the version apt would install for the package — the
// "Candidate:" line of apt-cache policy. That is the exact version a pinned
// install targets.
func aptCandidateVersion(packageName string) (string, error) {
	out, err := exec.Command("apt-cache", "policy", packageName).Output()
	if err != nil {
		return "", fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_apt_policy pkg=%s error=%w", packageName, err)
	}
	v := parseAptCandidate(string(out))
	if v == "" {
		return "", fmt.Errorf("[ERROR] [agent] [installer] hash_resolve_apt_no_candidate pkg=%s", packageName)
	}
	return v, nil
}

// parseAptCandidate extracts the installable version from `apt-cache policy`
// output ("Candidate:" line). Returns "" when absent or "(none)".
func parseAptCandidate(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			if v == "(none)" {
				return ""
			}
			return v
		}
	}
	return ""
}

// aptFieldFromStanza returns the value of the first matching field prefix in an
// apt-cache show stanza (e.g. "SHA256:").
func aptFieldFromStanza(output, prefix string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// fileSHA256 returns the lowercase hex SHA256 of a file, streaming.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
