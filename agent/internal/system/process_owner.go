package system

import "strings"

// ProcessOwner names what a process belongs to, read from its cgroup path.
// It is the join between the process list and the service, container, and
// package inventories — a PID answers "who owns me" without the Desktop
// having to guess from a name.
type ProcessOwner struct {
	Cgroup           string `json:"cgroup,omitempty"`
	Unit             string `json:"unit,omitempty"`
	ContainerID      string `json:"container_id,omitempty"`
	ContainerRuntime string `json:"container_runtime,omitempty"`
	PackageManager   string `json:"package_manager,omitempty"`
	PackageName      string `json:"package_name,omitempty"`
}

// parseCgroupOwner reads the contents of /proc/[pid]/cgroup.
//
// v2 is one `0::<path>` line. v1 is one line per controller, and the paths can
// disagree — the systemd hierarchy is the one that carries unit and container
// scopes, so it wins when present.
func parseCgroupOwner(content string) ProcessOwner {
	path := ""
	for _, line := range strings.Split(content, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(fields) != 3 || fields[2] == "" {
			continue
		}
		hierarchy, controllers, candidate := fields[0], fields[1], fields[2]
		if hierarchy == "0" && controllers == "" {
			path = candidate // v2: unified, authoritative
			break
		}
		if controllers == "name=systemd" {
			path = candidate
			continue
		}
		if path == "" && candidate != "/" {
			path = candidate
		}
	}
	if path == "" || path == "/" {
		return ProcessOwner{}
	}

	owner := ProcessOwner{Cgroup: path}
	segments := strings.Split(strings.Trim(path, "/"), "/")

	containerIndex := -1
	for i := len(segments) - 1; i >= 0; i-- {
		runtime, id := containerSegment(segments, i)
		if id == "" {
			continue
		}
		owner.ContainerRuntime = runtime
		owner.ContainerID = id
		containerIndex = i
		break
	}

	for i := len(segments) - 1; i >= 0; i-- {
		if i == containerIndex {
			continue
		}
		if strings.HasSuffix(segments[i], ".service") || strings.HasSuffix(segments[i], ".scope") {
			owner.Unit = segments[i]
			break
		}
	}
	return owner
}

// containerSegment identifies a container from one cgroup path segment,
// returning its runtime and full ID. Empty ID means the segment names no
// container.
func containerSegment(segments []string, i int) (string, string) {
	segment := segments[i]

	// systemd cgroup drivers: docker-<id>.scope, libpod-<id>.scope,
	// crio-<id>.scope, cri-containerd-<id>.scope.
	if scope := strings.TrimSuffix(segment, ".scope"); scope != segment {
		for prefix, runtime := range map[string]string{
			"docker-":         "docker",
			"libpod-":         "podman",
			"crio-":           "crio",
			"cri-containerd-": "containerd",
			"containerd-":     "containerd",
		} {
			if id := strings.TrimPrefix(scope, prefix); id != scope && isContainerID(id) {
				return runtime, id
			}
		}
	}

	// LXC keeps a name, not a hash: /lxc/<name>, /lxc.payload.<name>.
	if payload := strings.TrimPrefix(segment, "lxc.payload."); payload != segment && payload != "" {
		return "lxc", payload
	}
	if i > 0 && segments[i-1] == "lxc" && segment != "" {
		return "lxc", segment
	}

	// cgroupfs drivers keep the bare ID: /docker/<id>, /kubepods/.../<id>.
	if isContainerID(segment) {
		if i > 0 {
			switch {
			case segments[i-1] == "docker":
				return "docker", segment
			case strings.HasPrefix(segments[i-1], "pod"), strings.HasPrefix(segments[i-1], "kubepods"):
				return "containerd", segment
			}
		}
	}
	return "", ""
}

// isContainerID reports whether a segment is a container hash. Runtimes emit
// the full 64-hex ID; some emit the 12-hex short form, and the cgroup is not
// the place to decide which one a UI should show.
func isContainerID(segment string) bool {
	if len(segment) != 64 && len(segment) != 12 {
		return false
	}
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
