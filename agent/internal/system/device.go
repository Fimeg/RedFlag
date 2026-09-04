package system

// device.go — DEVICE-001: device form-factor detection.
//
// Classifies the host as container / vm / server / desktop / laptop / phone /
// tablet. The agent runs as a systemd service, so session environment
// (DISPLAY, WAYLAND_DISPLAY) is useless here — every signal is read from
// /sys and /proc. The server stores this as device_type; the operator can
// override it (device_type_manual), so misclassification is recoverable.
//
// Layered by signal reliability (DEVICE-001):
//
//	1. container — PID 1's environment carries container=, or systemd
//	   stamped /run/systemd/container (LXC, LXD, nspawn, docker, podman)
//	2. vm — DMI sys_vendor names a hypervisor; Hyper-V hides behind
//	   "Microsoft Corporation" (shared with Surface hardware) and is
//	   matched on product_name instead
//	3. SMBIOS chassis_type — the ACPI enum hostnamectl and osquery key
//	   off: laptop / desktop / server / tablet / phone by chassis class
//	4. DMI present but chassis unmapped — battery means laptop, not
//	   phone: phones don't ship SMBIOS at all
//	5. no DMI (ARM/embedded) — battery × display matrix, phone/tablet
//	   split on the framebuffer's short edge

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// phoneMaxMinDimension is the phone/tablet split on the framebuffer's smaller
// dimension in pixels. Phones run 720–1440 on the short edge; tablets start
// around 1600. The task's raw width<1024 test misfires on any modern phone
// (Pixel 3 is 1080 wide in portrait), so we compare the minimum dimension.
const phoneMaxMinDimension = 1440

// detectPaths carries every filesystem signal the classifier reads, so tests
// can point each one at a mock tree.
type detectPaths struct {
	pid1Environ      string // /proc/1/environ
	systemdContainer string // /run/systemd/container
	sysVendor        string // /sys/class/dmi/id/sys_vendor
	productName      string // /sys/class/dmi/id/product_name
	chassisType      string // /sys/class/dmi/id/chassis_type
	powerSupplyDir   string // /sys/class/power_supply
	drmDir           string // /sys/class/drm
	fbSizePath       string // /sys/class/graphics/fb0/virtual_size
}

// DetectDeviceType classifies this host's form factor. Only Linux exposes the
// signals we read; other platforms return "" and the server applies its
// conservative 'server' default.
func DetectDeviceType() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	return detectDeviceTypeFrom(detectPaths{
		pid1Environ:      "/proc/1/environ",
		systemdContainer: "/run/systemd/container",
		sysVendor:        "/sys/class/dmi/id/sys_vendor",
		productName:      "/sys/class/dmi/id/product_name",
		chassisType:      "/sys/class/dmi/id/chassis_type",
		powerSupplyDir:   "/sys/class/power_supply",
		drmDir:           "/sys/class/drm",
		fbSizePath:       "/sys/class/graphics/fb0/virtual_size",
	})
}

func detectDeviceTypeFrom(p detectPaths) string {
	// A container on a VM (or a laptop host) is still a container — the
	// agent manages the guest, not the hardware underneath it.
	if isContainer(p.pid1Environ, p.systemdContainer) {
		return "container"
	}
	if isVM(p.sysVendor, p.productName) {
		return "vm"
	}
	if t := chassisDeviceType(p.chassisType); t != "" {
		return t
	}

	battery := hasSystemBattery(p.powerSupplyDir)
	display := hasDisplay(p.drmDir, p.fbSizePath)

	if hasDMI(p.chassisType, p.sysVendor) {
		// x86-class firmware with an unknown/other chassis code. A battery
		// here means laptop (possibly lid-closed, hence display optional);
		// phones never expose SMBIOS.
		switch {
		case battery:
			return "laptop"
		case display:
			return "desktop"
		}
		return "server"
	}

	// No DMI: ARM/embedded (Pixel 3, Raspberry Pi, SBCs).
	switch {
	case !battery && !display:
		return "server"
	case !battery && display:
		return "desktop"
	case battery && !display:
		return "phone"
	}

	// battery + display: split phone/tablet on screen size
	if w, h := framebufferSize(p.fbSizePath); w > 0 && h > 0 {
		if min(w, h) < phoneMaxMinDimension {
			return "phone"
		}
		return "tablet"
	}
	return "phone"
}

// isContainer detects LXC/LXD/nspawn/docker/podman. PID 1's environment
// carries container= (readable thanks to the unit's CAP_SYS_PTRACE); when
// that read is denied, systemd's own detection at /run/systemd/container is
// the fallback.
func isContainer(pid1Environ, systemdContainer string) bool {
	if data, err := os.ReadFile(pid1Environ); err == nil {
		for _, kv := range strings.Split(string(data), "\x00") {
			if v, ok := strings.CutPrefix(kv, "container="); ok && v != "" {
				return true
			}
		}
	}
	if data, err := os.ReadFile(systemdContainer); err == nil {
		return strings.TrimSpace(string(data)) != ""
	}
	return false
}

// vmVendorPrefixes are DMI sys_vendor values only hypervisors emit.
// Microsoft is deliberately absent — Surface hardware shares that vendor
// string — and Google is absent because Chromebooks report GOOGLE; both are
// matched on product_name below.
var vmVendorPrefixes = []string{
	"QEMU", "VMware", "innotek GmbH", "Xen", "Bochs", "Parallels",
	"Amazon EC2", "DigitalOcean", "OpenStack Foundation", "oVirt",
	"Nutanix", "Red Hat", "Alibaba Cloud",
}

var vmProductNames = []string{
	"Virtual Machine",       // Hyper-V
	"Google Compute Engine", // GCE
	"KVM",
	"VirtualBox",
}

func isVM(sysVendorPath, productNamePath string) bool {
	if vendor := readTrimmed(sysVendorPath); vendor != "" {
		for _, prefix := range vmVendorPrefixes {
			if strings.HasPrefix(vendor, prefix) {
				return true
			}
		}
	}
	if product := readTrimmed(productNamePath); product != "" {
		for _, name := range vmProductNames {
			if product == name {
				return true
			}
		}
	}
	return false
}

// chassisDeviceType maps the SMBIOS chassis enum (SMBIOS spec 7.4.1) to a
// device type. Unknown/other codes return "" and fall through to the
// battery/display layers.
func chassisDeviceType(path string) string {
	code, err := strconv.Atoi(readTrimmed(path))
	if err != nil {
		return ""
	}
	switch code {
	case 8, 9, 10, 14, 31, 32: // portable, laptop, notebook, sub-notebook, convertible, detachable
		return "laptop"
	case 3, 4, 5, 6, 7, 13, 15, 16, 24, 34, 35, 36: // desktop through stick PC
		return "desktop"
	case 17, 23, 25, 28, 29: // main server, rack mount, multi-system, blade, blade enclosure
		return "server"
	case 11: // hand held
		return "phone"
	case 30:
		return "tablet"
	}
	return ""
}

// hasDMI reports whether SMBIOS/DMI firmware data exists at all. Its absence
// is itself a signal: ARM phones and SBCs have device-tree instead.
func hasDMI(chassisPath, sysVendorPath string) bool {
	if _, err := os.Stat(chassisPath); err == nil {
		return true
	}
	if _, err := os.Stat(sysVendorPath); err == nil {
		return true
	}
	return false
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// hasSystemBattery scans /sys/class/power_supply for a system battery.
// Peripheral batteries (bluetooth mice, keyboards) advertise scope=Device
// and must not classify a desktop as mobile; UPS units report type=UPS.
func hasSystemBattery(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		typ, err := os.ReadFile(filepath.Join(dir, e.Name(), "type"))
		if err != nil || strings.TrimSpace(string(typ)) != "Battery" {
			continue
		}
		if scope, err := os.ReadFile(filepath.Join(dir, e.Name(), "scope")); err == nil {
			if strings.TrimSpace(string(scope)) == "Device" {
				continue
			}
		}
		return true
	}
	return false
}

// hasDisplay reports whether a display is attached: any DRM connector in
// state "connected", falling back to a present framebuffer.
func hasDisplay(drmDir, fbSizePath string) bool {
	if entries, err := os.ReadDir(drmDir); err == nil {
		for _, e := range entries {
			status, err := os.ReadFile(filepath.Join(drmDir, e.Name(), "status"))
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(status)) == "connected" {
				return true
			}
		}
	}
	if _, err := os.Stat(fbSizePath); err == nil {
		return true
	}
	return false
}

// framebufferSize parses /sys/class/graphics/fb0/virtual_size ("1080,2160").
func framebufferSize(path string) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), ",", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil {
		return 0, 0
	}
	return w, h
}

// ReadDeviceModel returns the hardware model: device-tree on ARM, DMI on x86.
func ReadDeviceModel() string {
	return readDeviceModelFrom("/proc/device-tree/model", "/sys/class/dmi/id/product_name")
}

func readDeviceModelFrom(dtPath, dmiPath string) string {
	if model, err := os.ReadFile(dtPath); err == nil {
		// device-tree strings are NUL-terminated
		if s := strings.TrimSpace(strings.Trim(string(model), "\x00")); s != "" {
			return s
		}
	}
	if model, err := os.ReadFile(dmiPath); err == nil {
		if s := strings.TrimSpace(string(model)); s != "" {
			return s
		}
	}
	return ""
}

// DetectOSDistro returns the distro ID from /etc/os-release ("arch",
// "fedora", "debian"). Empty on non-Linux or when unreadable.
func DetectOSDistro() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	return osDistroFrom("/etc/os-release")
}

func osDistroFrom(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
	}
	return ""
}
