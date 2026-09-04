//go:build linux

package system

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var errSoftwareManagerUnavailable = errors.New("software manager unavailable")

type softwareCollector struct {
	name      string
	binary    string
	inventory func() ([]InstalledPackage, error)
	detail    func(string) (*PackageDetail, error)
	owner     func(string) (*SoftwareOwner, error)
}

func linuxSoftwareCollectors() []softwareCollector {
	return []softwareCollector{
		{name: "pacman", binary: "pacman", inventory: pacmanInventory, detail: pacmanDetail, owner: pacmanOwner},
		{name: "apt", binary: "dpkg-query", inventory: dpkgInventory, detail: dpkgDetail, owner: dpkgOwner},
		{name: "dnf", binary: "rpm", inventory: rpmInventory, detail: rpmDetail, owner: rpmOwner},
	}
}

func getSoftwareSnapshot() (*SoftwareSnapshot, error) {
	snapshot := &SoftwareSnapshot{Supported: true, CollectedAt: time.Now().UTC()}
	for _, collector := range linuxSoftwareCollectors() {
		state := SoftwareManager{Name: collector.name}
		if _, err := exec.LookPath(collector.binary); err != nil {
			snapshot.Managers = append(snapshot.Managers, state)
			continue
		}
		state.Available = true
		packages, err := collector.inventory()
		if err != nil {
			state.Error = conciseCommandError(err)
			snapshot.Managers = append(snapshot.Managers, state)
			continue
		}
		state.Count = len(packages)
		snapshot.Managers = append(snapshot.Managers, state)
		snapshot.Packages = append(snapshot.Packages, packages...)
	}

	sort.Slice(snapshot.Packages, func(i, j int) bool {
		if snapshot.Packages[i].Name != snapshot.Packages[j].Name {
			return snapshot.Packages[i].Name < snapshot.Packages[j].Name
		}
		return snapshot.Packages[i].PackageType < snapshot.Packages[j].PackageType
	})
	snapshot.Count = len(snapshot.Packages)
	for _, pkg := range snapshot.Packages {
		switch pkg.InstallReason {
		case "explicit":
			snapshot.ExplicitCount++
		case "dependency":
			snapshot.DependencyCount++
		}
		if pkg.Origin == "foreign" {
			snapshot.ForeignCount++
		}
	}
	return snapshot, nil
}

func getPackageDetail(packageType, identity string) (*PackageDetail, error) {
	if !validPackageIdentity(identity) {
		return nil, fmt.Errorf("invalid package identity")
	}
	for _, collector := range linuxSoftwareCollectors() {
		if collector.name != packageType {
			continue
		}
		if _, err := exec.LookPath(collector.binary); err != nil {
			return nil, fmt.Errorf("%s: %w", packageType, errSoftwareManagerUnavailable)
		}
		return collector.detail(identity)
	}
	return nil, fmt.Errorf("unsupported package manager %q", packageType)
}

func findSoftwareOwner(path string) (*SoftwareOwner, error) {
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsRune(path, '\x00') {
		return nil, fmt.Errorf("invalid executable path")
	}
	var lastErr error
	for _, collector := range linuxSoftwareCollectors() {
		if _, err := exec.LookPath(collector.binary); err != nil {
			continue
		}
		owner, err := collector.owner(path)
		if err == nil && owner != nil {
			return owner, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errSoftwareManagerUnavailable
	}
	return nil, lastErr
}

func packageCommand(binary string, args ...string) ([]byte, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", binary, errSoftwareManagerUnavailable)
	}
	command := exec.Command(path, args...)
	command.Env = []string{
		"LC_ALL=C",
		"LANG=C",
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
	}
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s query failed: %w", binary, err)
	}
	return output, nil
}

func conciseCommandError(err error) string {
	if errors.Is(err, errSoftwareManagerUnavailable) {
		return "unavailable"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "query exited " + strconv.Itoa(exitErr.ExitCode())
	}
	return err.Error()
}

func validPackageIdentity(identity string) bool {
	if identity == "" || len(identity) > 255 || (!unicode.IsLetter(rune(identity[0])) && !unicode.IsDigit(rune(identity[0]))) {
		return false
	}
	for _, char := range identity {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("@+_.:-", char) {
			continue
		}
		return false
	}
	return true
}

func lines(output []byte) []string {
	var result []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		fields := strings.Fields(value)
		if len(fields) > 0 {
			set[fields[0]] = true
		}
	}
	return set
}

func splitPackageList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "None" || value == "(none)" {
		return nil
	}
	fields := strings.Fields(value)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(strings.TrimSuffix(field, ","))
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

func parseColonRecord(output []byte) map[string]string {
	record := make(map[string]string)
	lastKey := ""
	for _, raw := range strings.Split(string(output), "\n") {
		if raw == "" {
			continue
		}
		if (raw[0] == ' ' || raw[0] == '\t') && lastKey != "" {
			continuation := strings.TrimSpace(raw)
			if continuation != "" {
				record[lastKey] = strings.TrimSpace(record[lastKey] + " " + continuation)
			}
			continue
		}
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			lastKey = ""
			continue
		}
		lastKey = strings.TrimSpace(parts[0])
		record[lastKey] = strings.TrimSpace(parts[1])
	}
	return record
}

func parseHumanSize(value string) uint64 {
	fields := strings.Fields(strings.ReplaceAll(value, ",", ""))
	if len(fields) == 0 {
		return 0
	}
	amount, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || amount < 0 {
		return 0
	}
	multiplier := float64(1)
	if len(fields) > 1 {
		switch strings.ToLower(fields[1]) {
		case "kib", "kb":
			multiplier = 1024
		case "mib", "mb":
			multiplier = 1024 * 1024
		case "gib", "gb":
			multiplier = 1024 * 1024 * 1024
		case "tib", "tb":
			multiplier = 1024 * 1024 * 1024 * 1024
		}
	}
	return uint64(amount * multiplier)
}

func boundedFiles(values []string) ([]string, int, bool) {
	count := len(values)
	if count <= MaxPackageFiles {
		return values, count, false
	}
	return values[:MaxPackageFiles], count, true
}

func pacmanInventory() ([]InstalledPackage, error) {
	installed, err := packageCommand("pacman", "-Q")
	if err != nil {
		return nil, err
	}
	explicitOutput, explicitErr := packageCommand("pacman", "-Qqe")
	dependencyOutput, dependencyErr := packageCommand("pacman", "-Qqd")
	foreignOutput, foreignErr := packageCommand("pacman", "-Qm")
	explicit := map[string]bool{}
	dependencies := map[string]bool{}
	foreign := map[string]bool{}
	if explicitErr == nil {
		explicit = stringSet(lines(explicitOutput))
	}
	if dependencyErr == nil {
		dependencies = stringSet(lines(dependencyOutput))
	}
	if foreignErr == nil {
		foreign = stringSet(lines(foreignOutput))
	}

	var packages []InstalledPackage
	for _, line := range lines(installed) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		reason := "unknown"
		switch {
		case explicit[fields[0]]:
			reason = "explicit"
		case dependencies[fields[0]]:
			reason = "dependency"
		}
		origin := "repository"
		if foreign[fields[0]] {
			origin = "foreign"
		}
		packages = append(packages, InstalledPackage{
			PackageType: "pacman", Identity: fields[0], Name: fields[0], Version: fields[1],
			InstallReason: reason, Origin: origin,
		})
	}
	return packages, nil
}

func pacmanDetail(identity string) (*PackageDetail, error) {
	output, err := packageCommand("pacman", "-Qi", "--color=never", identity)
	if err != nil {
		return nil, err
	}
	record := parseColonRecord(output)
	origin := "repository"
	if foreign, err := packageCommand("pacman", "-Qm", identity); err == nil && len(lines(foreign)) > 0 {
		origin = "foreign"
	}
	reason := "unknown"
	if strings.EqualFold(record["Install Reason"], "Explicitly installed") {
		reason = "explicit"
	} else if strings.Contains(strings.ToLower(record["Install Reason"]), "dependency") {
		reason = "dependency"
	}
	detail := &PackageDetail{InstalledPackage: InstalledPackage{
		PackageType: "pacman", Identity: identity, Name: record["Name"], Version: record["Version"],
		Architecture: record["Architecture"], Description: record["Description"], InstallReason: reason,
		Origin: origin, InstalledSizeBytes: parseHumanSize(record["Installed Size"]), InstalledAt: record["Install Date"],
	}, URL: record["URL"], Licenses: splitPackageList(record["Licenses"]), Groups: splitPackageList(record["Groups"]),
		DependsOn: splitPackageList(record["Depends On"]), OptionalDependencies: splitPackageList(record["Optional Deps"]),
		RequiredBy: splitPackageList(record["Required By"]), Provides: splitPackageList(record["Provides"]),
		ConflictsWith: splitPackageList(record["Conflicts With"]), Replaces: splitPackageList(record["Replaces"]),
		Packager: record["Packager"], BuildDate: record["Build Date"],
	}
	if detail.Name == "" {
		detail.Name = identity
	}
	if output, err := packageCommand("pacman", "-Ql", identity); err == nil {
		prefix := identity + " "
		var files []string
		for _, line := range lines(output) {
			files = append(files, strings.TrimPrefix(line, prefix))
		}
		detail.Files, detail.FileCount, detail.FilesTruncated = boundedFiles(files)
	}
	return detail, nil
}

func pacmanOwner(path string) (*SoftwareOwner, error) {
	output, err := packageCommand("pacman", "-Qoq", path)
	if err != nil {
		return nil, err
	}
	names := lines(output)
	if len(names) == 0 {
		return nil, fmt.Errorf("no pacman package owns path")
	}
	return &SoftwareOwner{PackageType: "pacman", PackageName: names[0]}, nil
}

func dpkgInventory() ([]InstalledPackage, error) {
	output, err := packageCommand("dpkg-query", "-W", "-f=${binary:Package}\t${Version}\t${Architecture}\t${db:Status-Abbrev}\t${binary:Synopsis}\n")
	if err != nil {
		return nil, err
	}
	manual := map[string]bool{}
	if manualOutput, err := packageCommand("apt-mark", "showmanual"); err == nil {
		manual = stringSet(lines(manualOutput))
	}
	var packages []InstalledPackage
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 5 || !strings.HasPrefix(fields[3], "ii") {
			continue
		}
		identity := fields[0]
		name := strings.TrimSuffix(identity, ":"+fields[2])
		reason := "unknown"
		if manual[name] || manual[identity] {
			reason = "explicit"
		} else if len(manual) > 0 {
			reason = "dependency"
		}
		packages = append(packages, InstalledPackage{
			PackageType: "apt", Identity: identity, Name: name, Version: fields[1], Architecture: fields[2],
			Description: fields[4], InstallReason: reason, Origin: "unknown",
		})
	}
	return packages, nil
}

func dpkgDetail(identity string) (*PackageDetail, error) {
	format := "${binary:Package}\t${Version}\t${Architecture}\t${binary:Synopsis}\t${Homepage}\t${Installed-Size}\t${Maintainer}\t${Depends}\t${Pre-Depends}\t${Recommends}\t${Suggests}\t${Provides}\t${Conflicts}\t${Replaces}\n"
	output, err := packageCommand("dpkg-query", "-W", "-f="+format, identity)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimRight(string(output), "\n"), "\t")
	if len(fields) < 14 {
		return nil, fmt.Errorf("dpkg returned incomplete package detail")
	}
	name := strings.TrimSuffix(fields[0], ":"+fields[2])
	reason := "unknown"
	if manual, err := packageCommand("apt-mark", "showmanual"); err == nil {
		set := stringSet(lines(manual))
		if set[name] || set[identity] {
			reason = "explicit"
		} else {
			reason = "dependency"
		}
	}
	installedSize, _ := strconv.ParseUint(strings.TrimSpace(fields[5]), 10, 64)
	detail := &PackageDetail{InstalledPackage: InstalledPackage{
		PackageType: "apt", Identity: fields[0], Name: name, Version: fields[1], Architecture: fields[2],
		Description: fields[3], InstallReason: reason, Origin: "unknown", InstalledSizeBytes: installedSize * 1024,
	}, URL: fields[4], Packager: fields[6]}
	detail.DependsOn = splitCommaDependencies(strings.Join([]string{fields[7], fields[8]}, ","))
	detail.OptionalDependencies = splitCommaDependencies(strings.Join([]string{fields[9], fields[10]}, ","))
	detail.Provides = splitCommaDependencies(fields[11])
	detail.ConflictsWith = splitCommaDependencies(fields[12])
	detail.Replaces = splitCommaDependencies(fields[13])
	if output, err := packageCommand("dpkg-query", "-L", identity); err == nil {
		detail.Files, detail.FileCount, detail.FilesTruncated = boundedFiles(lines(output))
	}
	return detail, nil
}

func splitCommaDependencies(value string) []string {
	var result []string
	for _, dependency := range strings.Split(value, ",") {
		dependency = strings.TrimSpace(dependency)
		if dependency != "" && dependency != "<none>" {
			result = append(result, dependency)
		}
	}
	return result
}

func dpkgOwner(path string) (*SoftwareOwner, error) {
	output, err := packageCommand("dpkg-query", "-S", path)
	if err != nil {
		return nil, err
	}
	line := strings.SplitN(string(output), "\n", 2)[0]
	parts := strings.SplitN(line, ": ", 2)
	if len(parts) != 2 || parts[0] == "" {
		return nil, fmt.Errorf("dpkg returned no package owner")
	}
	name := parts[0]
	if index := strings.LastIndex(name, ":"); index > 0 {
		name = name[:index]
	}
	return &SoftwareOwner{PackageType: "apt", PackageName: name}, nil
}

func rpmInventory() ([]InstalledPackage, error) {
	format := "%{NAME}\t%{NEVRA}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SUMMARY}\t%{SIZE}\t%{INSTALLTIME}\n"
	output, err := packageCommand("rpm", "-qa", "--qf", format)
	if err != nil {
		return nil, err
	}
	var packages []InstalledPackage
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		size, _ := strconv.ParseUint(fields[5], 10, 64)
		installed := ""
		if epoch, err := strconv.ParseInt(fields[6], 10, 64); err == nil && epoch > 0 {
			installed = time.Unix(epoch, 0).UTC().Format(time.RFC3339)
		}
		packages = append(packages, InstalledPackage{
			PackageType: "dnf", Identity: fields[1], Name: fields[0], Version: fields[2], Architecture: fields[3],
			Description: fields[4], InstallReason: "unknown", Origin: "unknown", InstalledSizeBytes: size, InstalledAt: installed,
		})
	}
	return packages, nil
}

func rpmDetail(identity string) (*PackageDetail, error) {
	output, err := packageCommand("rpm", "-qi", identity)
	if err != nil {
		return nil, err
	}
	record := parseColonRecord(output)
	version := record["Version"]
	if release := record["Release"]; release != "" {
		version += "-" + release
	}
	detail := &PackageDetail{InstalledPackage: InstalledPackage{
		PackageType: "dnf", Identity: identity, Name: record["Name"], Version: version,
		Architecture: record["Architecture"], Description: record["Summary"], InstallReason: "unknown",
		Origin: "unknown", InstalledSizeBytes: parseHumanSize(record["Size"]), InstalledAt: record["Install Date"],
	}, URL: record["URL"], Licenses: splitPackageList(record["License"]), Packager: record["Packager"], BuildDate: record["Build Date"]}
	if output, err := packageCommand("rpm", "-qR", identity); err == nil {
		for _, requirement := range lines(output) {
			if !strings.HasPrefix(requirement, "rpmlib(") {
				detail.DependsOn = append(detail.DependsOn, requirement)
			}
		}
	}
	if output, err := packageCommand("rpm", "-q", "--provides", identity); err == nil {
		detail.Provides = lines(output)
	}
	if output, err := packageCommand("rpm", "-q", "--whatrequires", detail.Name); err == nil {
		detail.RequiredBy = lines(output)
	}
	if output, err := packageCommand("rpm", "-ql", identity); err == nil {
		detail.Files, detail.FileCount, detail.FilesTruncated = boundedFiles(lines(output))
	}
	return detail, nil
}

func rpmOwner(path string) (*SoftwareOwner, error) {
	output, err := packageCommand("rpm", "-qf", "--qf", "%{NAME}\n", path)
	if err != nil {
		return nil, err
	}
	names := lines(output)
	if len(names) == 0 {
		return nil, fmt.Errorf("rpm returned no package owner")
	}
	return &SoftwareOwner{PackageType: "dnf", PackageName: names[0]}, nil
}
