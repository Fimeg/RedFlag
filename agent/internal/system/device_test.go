package system

// device_test.go — DEVICE-001: form-factor detection against mocked /sys trees.

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeSysTree builds a mock tree for every signal the classifier reads. DMI
// and container paths point at nonexistent files until a test writes them,
// which is exactly the ARM/bare-metal shape.
type fakeSysTree struct {
	pid1Environ      string
	systemdContainer string
	sysVendor        string
	productName      string
	chassisType      string
	powerDir         string
	drmDir           string
	fbSize           string
}

func newFakeSysTree(t *testing.T) *fakeSysTree {
	t.Helper()
	root := t.TempDir()
	tree := &fakeSysTree{
		pid1Environ:      filepath.Join(root, "pid1_environ"),
		systemdContainer: filepath.Join(root, "systemd_container"),
		sysVendor:        filepath.Join(root, "sys_vendor"),
		productName:      filepath.Join(root, "product_name"),
		chassisType:      filepath.Join(root, "chassis_type"),
		powerDir:         filepath.Join(root, "power_supply"),
		drmDir:           filepath.Join(root, "drm"),
		fbSize:           filepath.Join(root, "fb0_virtual_size"),
	}
	if err := os.MkdirAll(tree.powerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tree.drmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return tree
}

func (f *fakeSysTree) addPowerSupply(t *testing.T, name, typ, scope string) {
	t.Helper()
	dir := filepath.Join(f.powerDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "type"), []byte(typ+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if scope != "" {
		if err := os.WriteFile(filepath.Join(dir, "scope"), []byte(scope+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *fakeSysTree) addConnector(t *testing.T, name, status string) {
	t.Helper()
	dir := filepath.Join(f.drmDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeSysTree) setFramebuffer(t *testing.T, size string) {
	t.Helper()
	if err := os.WriteFile(f.fbSize, []byte(size+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeSysTree) writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeSysTree) setDMI(t *testing.T, vendor, product, chassis string) {
	t.Helper()
	if vendor != "" {
		f.writeFile(t, f.sysVendor, vendor+"\n")
	}
	if product != "" {
		f.writeFile(t, f.productName, product+"\n")
	}
	if chassis != "" {
		f.writeFile(t, f.chassisType, chassis+"\n")
	}
}

func (f *fakeSysTree) detect() string {
	return detectDeviceTypeFrom(detectPaths{
		pid1Environ:      f.pid1Environ,
		systemdContainer: f.systemdContainer,
		sysVendor:        f.sysVendor,
		productName:      f.productName,
		chassisType:      f.chassisType,
		powerSupplyDir:   f.powerDir,
		drmDir:           f.drmDir,
		fbSizePath:       f.fbSize,
	})
}

func TestDetectServer(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.addPowerSupply(t, "AC", "Mains", "")
	tree.addConnector(t, "card0-VGA-1", "disconnected")

	if got := tree.detect(); got != "server" {
		t.Errorf("[ERROR] [agent] [system] no battery, no display: got %q, want server", got)
	}
}

func TestDetectDesktop(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.addConnector(t, "card0-HDMI-A-1", "connected")

	if got := tree.detect(); got != "desktop" {
		t.Errorf("[ERROR] [agent] [system] display, no battery: got %q, want desktop", got)
	}
}

func TestDetectPhonePixel3(t *testing.T) {
	// Pixel 3 blueline: system battery, panel connected, 1080x2160 portrait
	// fb, no DMI (device-tree platform)
	tree := newFakeSysTree(t)
	tree.addPowerSupply(t, "battery", "Battery", "System")
	tree.addConnector(t, "card0-DSI-1", "connected")
	tree.setFramebuffer(t, "1080,2160")

	if got := tree.detect(); got != "phone" {
		t.Errorf("[ERROR] [agent] [system] Pixel 3 signals: got %q, want phone", got)
	}
}

func TestDetectTablet(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.addPowerSupply(t, "battery", "Battery", "")
	tree.addConnector(t, "card0-DSI-1", "connected")
	tree.setFramebuffer(t, "2560,1600")

	if got := tree.detect(); got != "tablet" {
		t.Errorf("[ERROR] [agent] [system] large battery-backed display: got %q, want tablet", got)
	}
}

func TestDetectBatteryNoDisplayIsPhone(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.addPowerSupply(t, "bms", "Battery", "")

	if got := tree.detect(); got != "phone" {
		t.Errorf("[ERROR] [agent] [system] battery, no display: got %q, want phone", got)
	}
}

func TestPeripheralBatteryDoesNotMakeDesktopMobile(t *testing.T) {
	// Bluetooth mouse battery has scope=Device; desktop must stay desktop
	tree := newFakeSysTree(t)
	tree.addPowerSupply(t, "hid-aa:bb-battery", "Battery", "Device")
	tree.addConnector(t, "card0-DP-1", "connected")

	if got := tree.detect(); got != "desktop" {
		t.Errorf("[ERROR] [agent] [system] peripheral battery misclassified: got %q, want desktop", got)
	}
}

func TestUPSDoesNotMakeServerMobile(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.addPowerSupply(t, "ups", "UPS", "")

	if got := tree.detect(); got != "server" {
		t.Errorf("[ERROR] [agent] [system] UPS misclassified: got %q, want server", got)
	}
}

func TestDetectLaptopChassis(t *testing.T) {
	// The live regression: Fedora laptop, battery + connected 1080p panel.
	// Pre-chassis logic classified this as phone (min dimension 1080 < 1440).
	tree := newFakeSysTree(t)
	tree.setDMI(t, "Dell Inc.", "Precision 5820", "10")
	tree.addPowerSupply(t, "BAT0", "Battery", "")
	tree.addConnector(t, "card0-eDP-1", "connected")
	tree.setFramebuffer(t, "1920,1080")

	if got := tree.detect(); got != "laptop" {
		t.Errorf("[ERROR] [agent] [system] chassis 10 notebook: got %q, want laptop", got)
	}
}

func TestDetectDesktopChassis(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.setDMI(t, "ASUS", "PRIME B550", "3")

	if got := tree.detect(); got != "desktop" {
		t.Errorf("[ERROR] [agent] [system] chassis 3 desktop: got %q, want desktop", got)
	}
}

func TestDetectServerChassisRackMount(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.setDMI(t, "Supermicro", "X11DPH-T", "23")
	tree.addConnector(t, "card0-VGA-1", "connected") // BMC display must not flip it to desktop

	if got := tree.detect(); got != "server" {
		t.Errorf("[ERROR] [agent] [system] chassis 23 rack mount: got %q, want server", got)
	}
}

func TestDetectTabletChassis(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.setDMI(t, "LENOVO", "MIIX 520", "30")

	if got := tree.detect(); got != "tablet" {
		t.Errorf("[ERROR] [agent] [system] chassis 30 tablet: got %q, want tablet", got)
	}
}

func TestUnknownChassisWithBatteryIsLaptop(t *testing.T) {
	// DMI present but chassis "Other" (1): a battery on SMBIOS hardware
	// means laptop — phones don't have SMBIOS at all.
	tree := newFakeSysTree(t)
	tree.setDMI(t, "Framework", "Laptop 13", "1")
	tree.addPowerSupply(t, "BAT1", "Battery", "")
	tree.addConnector(t, "card0-eDP-1", "connected")
	tree.setFramebuffer(t, "2256,1504")

	if got := tree.detect(); got != "laptop" {
		t.Errorf("[ERROR] [agent] [system] unknown chassis + battery + DMI: got %q, want laptop", got)
	}
}

func TestDetectVMQemu(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.setDMI(t, "QEMU", "Standard PC (Q35 + ICH9, 2009)", "1")
	tree.addConnector(t, "card0-Virtual-1", "connected")

	if got := tree.detect(); got != "vm" {
		t.Errorf("[ERROR] [agent] [system] QEMU vendor: got %q, want vm", got)
	}
}

func TestDetectVMHyperV(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.setDMI(t, "Microsoft Corporation", "Virtual Machine", "3")

	if got := tree.detect(); got != "vm" {
		t.Errorf("[ERROR] [agent] [system] Hyper-V product name: got %q, want vm", got)
	}
}

func TestSurfaceHardwareIsNotVM(t *testing.T) {
	// Surface shares sys_vendor with Hyper-V guests; product_name is the
	// disambiguator and the chassis code must win here.
	tree := newFakeSysTree(t)
	tree.setDMI(t, "Microsoft Corporation", "Surface Pro 9", "9")
	tree.addPowerSupply(t, "BAT0", "Battery", "")

	if got := tree.detect(); got != "laptop" {
		t.Errorf("[ERROR] [agent] [system] Surface misclassified: got %q, want laptop", got)
	}
}

func TestDetectContainerFromPid1Environ(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.writeFile(t, tree.pid1Environ, "PATH=/usr/bin\x00container=lxc\x00HOME=/root")
	// LXC exposes the host's DMI; container must win over vm/chassis
	tree.setDMI(t, "QEMU", "Standard PC (Q35 + ICH9, 2009)", "1")

	if got := tree.detect(); got != "container" {
		t.Errorf("[ERROR] [agent] [system] container=lxc in pid1 environ: got %q, want container", got)
	}
}

func TestDetectContainerFromSystemdFile(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.writeFile(t, tree.systemdContainer, "lxc\n")

	if got := tree.detect(); got != "container" {
		t.Errorf("[ERROR] [agent] [system] /run/systemd/container: got %q, want container", got)
	}
}

func TestEmptyContainerVarIsNotContainer(t *testing.T) {
	tree := newFakeSysTree(t)
	tree.writeFile(t, tree.pid1Environ, "PATH=/usr/bin\x00container=\x00HOME=/root")
	tree.setDMI(t, "ASUS", "PRIME B550", "3")

	if got := tree.detect(); got != "desktop" {
		t.Errorf("[ERROR] [agent] [system] empty container= treated as container: got %q, want desktop", got)
	}
}

func TestReadDeviceModelPrefersDeviceTree(t *testing.T) {
	dir := t.TempDir()
	dt := filepath.Join(dir, "model")
	dmi := filepath.Join(dir, "product_name")
	if err := os.WriteFile(dt, []byte("Google Pixel 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dmi, []byte("Precision 5820\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := readDeviceModelFrom(dt, dmi); got != "Google Pixel 3" {
		t.Errorf("[ERROR] [agent] [system] got %q, want device-tree model", got)
	}
	if got := readDeviceModelFrom(filepath.Join(dir, "missing"), dmi); got != "Precision 5820" {
		t.Errorf("[ERROR] [agent] [system] got %q, want DMI fallback", got)
	}
}

func TestOSDistroParsesID(t *testing.T) {
	dir := t.TempDir()
	osRelease := filepath.Join(dir, "os-release")
	content := "NAME=\"Arch Linux ARM\"\nPRETTY_NAME=\"Arch Linux ARM\"\nID=archarm\nID_LIKE=arch\n"
	if err := os.WriteFile(osRelease, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := osDistroFrom(osRelease); got != "archarm" {
		t.Errorf("[ERROR] [agent] [system] got %q, want archarm", got)
	}
}
