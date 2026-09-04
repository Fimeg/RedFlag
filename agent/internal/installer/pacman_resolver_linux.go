//go:build linux

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

const pacmanCommandPath = "/usr/bin/pacman"

var pacmanResolverEnv = []string{
	"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	"LC_ALL=C",
	"LANG=C",
}

func pacmanResolverCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = pacmanResolverEnv
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 4096 {
			message = message[:4096]
		}
		return nil, fmt.Errorf("%s failed: %w: %s", filepath.Base(name), err, message)
	}
	return output, nil
}

func parsePacmanPrint(output string) (map[string]string, error) {
	repositories := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 3 || fields[0] == "" || fields[1] == "" || fields[2] == "" {
			return nil, fmt.Errorf("pacman printed malformed closure identity %q", line)
		}
		repositories[fields[0]+"@"+fields[1]] = fields[2]
	}
	if len(repositories) == 0 {
		return nil, fmt.Errorf("pacman resolved an empty transaction")
	}
	return repositories, nil
}

func inspectPacmanArchive(path string) (string, string, error) {
	output, err := pacmanResolverCommand(pacmanCommandPath, "-Qp", "--", path)
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
		return "", "", fmt.Errorf("pacman printed malformed archive identity for %s", filepath.Base(path))
	}
	return fields[0], fields[1], nil
}

func regularPacmanArtifact(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}
	return nil
}

// ResolvePacmanClosure refreshes a private sync database, resolves the exact
// transaction, and downloads its archives plus detached signatures into an
// Agent-owned cache. It never mutates the host pacman database or package set.
func ResolvePacmanClosure(packageName, version string) (*PacmanResolution, error) {
	if err := validatePackageName(packageName); err != nil {
		return nil, err
	}
	if version != "" {
		if err := validatePackageName(version); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(pacmanCommandPath); err != nil {
		return nil, fmt.Errorf("pacman is unavailable: %w", err)
	}
	if _, err := exec.LookPath("fakeroot"); err != nil {
		return nil, fmt.Errorf("fakeroot is required for private pacman resolution: %w", err)
	}

	agentDir := filepath.Join(constants.GetBaseDir(), constants.AgentDir)
	resolutionRoot := filepath.Join(agentDir, "pacman-resolve")
	if err := os.MkdirAll(resolutionRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create pacman resolution root: %w", err)
	}
	root, err := os.MkdirTemp(resolutionRoot, "operation-")
	if err != nil {
		return nil, fmt.Errorf("create pacman resolution: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	fail := func(err error) (*PacmanResolution, error) {
		cleanup()
		return nil, err
	}

	database := filepath.Join(root, "db")
	cache := filepath.Join(root, "cache")
	if err := os.Mkdir(database, 0o700); err != nil {
		return fail(fmt.Errorf("create pacman database: %w", err))
	}
	if err := os.Mkdir(cache, 0o700); err != nil {
		return fail(fmt.Errorf("create pacman cache: %w", err))
	}
	if err := os.Symlink("/var/lib/pacman/local", filepath.Join(database, "local")); err != nil {
		return fail(fmt.Errorf("bind installed pacman state: %w", err))
	}

	fakeroot, err := exec.LookPath("fakeroot")
	if err != nil {
		return fail(err)
	}
	if _, err := pacmanResolverCommand(
		fakeroot, "--", pacmanCommandPath, "-Sy", "--noconfirm",
		"--disable-sandbox-filesystem", "--dbpath", database, "--logfile", "/dev/null",
	); err != nil {
		return fail(fmt.Errorf("refresh private pacman metadata: %w", err))
	}

	target := packageName
	if version != "" {
		target += "=" + version
	}
	printOutput, err := pacmanResolverCommand(
		pacmanCommandPath, "-Sp", "--print-format", "%n|%v|%r",
		"--dbpath", database, "--", target,
	)
	if err != nil {
		return fail(fmt.Errorf("resolve pacman transaction: %w", err))
	}
	repositories, err := parsePacmanPrint(string(printOutput))
	if err != nil {
		return fail(err)
	}
	if _, err := pacmanResolverCommand(
		fakeroot, "--", pacmanCommandPath, "-Sw", "--noconfirm", "--needed",
		"--disable-sandbox-filesystem", "--dbpath", database, "--cachedir", cache,
		"--logfile", "/dev/null", "--", target,
	); err != nil {
		return fail(fmt.Errorf("download pacman transaction: %w", err))
	}

	entries, err := os.ReadDir(cache)
	if err != nil {
		return fail(fmt.Errorf("read pacman cache: %w", err))
	}
	artifacts := make([]PacmanResolvedArtifact, 0, len(entries)/2)
	rootFound := false
	for _, entry := range entries {
		name := entry.Name()
		if !strings.Contains(name, ".pkg.tar.") || strings.HasSuffix(name, ".sig") || strings.HasSuffix(name, ".part") {
			continue
		}
		archivePath := filepath.Join(cache, name)
		if err := regularPacmanArtifact(archivePath); err != nil {
			return fail(fmt.Errorf("unsafe pacman archive %s: %w", name, err))
		}
		signaturePath := archivePath + ".sig"
		if err := regularPacmanArtifact(signaturePath); err != nil {
			return fail(fmt.Errorf("pacman archive %s has no regular detached signature: %w", name, err))
		}
		resolvedName, resolvedVersion, err := inspectPacmanArchive(archivePath)
		if err != nil {
			return fail(err)
		}
		repository, ok := repositories[resolvedName+"@"+resolvedVersion]
		if !ok {
			return fail(fmt.Errorf("pacman repository missing for %s@%s", resolvedName, resolvedVersion))
		}
		archiveHash, err := fileSHA256(archivePath)
		if err != nil {
			return fail(fmt.Errorf("hash pacman archive %s: %w", name, err))
		}
		signatureHash, err := fileSHA256(signaturePath)
		if err != nil {
			return fail(fmt.Errorf("hash pacman signature %s: %w", filepath.Base(signaturePath), err))
		}
		artifacts = append(artifacts, PacmanResolvedArtifact{
			Name: resolvedName, Version: resolvedVersion, Repository: repository,
			ArchivePath: archivePath, ArchiveSHA256: archiveHash,
			SignaturePath: signaturePath, SignatureSHA256: signatureHash,
		})
		if resolvedName == packageName && (version == "" || resolvedVersion == version) {
			rootFound = true
		}
	}
	if len(artifacts) == 0 {
		return fail(fmt.Errorf("pacman downloaded no package archives"))
	}
	if !rootFound {
		return fail(fmt.Errorf("pacman transaction did not contain requested root %s@%s", packageName, version))
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Name == artifacts[j].Name {
			return artifacts[i].Version < artifacts[j].Version
		}
		return artifacts[i].Name < artifacts[j].Name
	})
	return &PacmanResolution{Artifacts: artifacts, Cleanup: cleanup}, nil
}
