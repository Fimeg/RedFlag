package system

// machine_id_arm_test.go — DEVICE-002: ARM machine ID fallback.
//
// ARM devices (phones, SBCs) have no /sys/class/dmi/id/product_uuid. The
// fallback chain gains two hardware-bound sources before the weak hostname
// fallback: device-tree model + /etc/machine-id, then /proc/cpuinfo Serial.

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestDeviceTreeMachineIDCombinesModelAndMachineID(t *testing.T) {
	dir := t.TempDir()
	// device-tree model strings are NUL-terminated
	model := writeTempFile(t, dir, "model", "Google Pixel 3\x00")
	machineID := writeTempFile(t, dir, "machine-id", "abcdef0123456789abcdef0123456789\n")

	got := deviceTreeMachineIDFrom(model, machineID)
	want := "Google Pixel 3:abcdef0123456789abcdef0123456789"
	if got != want {
		t.Errorf("[ERROR] [agent] [system] got %q, want %q", got, want)
	}
}

func TestDeviceTreeMachineIDWithoutMachineIDIsHardwareBound(t *testing.T) {
	dir := t.TempDir()
	model := writeTempFile(t, dir, "model", "Raspberry Pi 4 Model B\x00")

	got := deviceTreeMachineIDFrom(model, filepath.Join(dir, "missing"))
	want := "Raspberry Pi 4 Model B:arm-device"
	if got != want {
		t.Errorf("[ERROR] [agent] [system] got %q, want %q", got, want)
	}
}

func TestDeviceTreeMachineIDAbsentOnX86(t *testing.T) {
	dir := t.TempDir()
	if got := deviceTreeMachineIDFrom(filepath.Join(dir, "missing"), filepath.Join(dir, "machine-id")); got != "" {
		t.Errorf("[ERROR] [agent] [system] expected empty on missing device-tree, got %q", got)
	}
}

func TestARMSerialParsed(t *testing.T) {
	dir := t.TempDir()
	cpuinfo := writeTempFile(t, dir, "cpuinfo",
		"processor\t: 0\nHardware\t: Qualcomm Technologies, Inc SDM845\nSerial\t\t: 0000000012345abc\n")

	got := armSerialFrom(cpuinfo)
	want := "arm-serial:0000000012345abc"
	if got != want {
		t.Errorf("[ERROR] [agent] [system] got %q, want %q", got, want)
	}
}

func TestARMSerialRejectsAllZero(t *testing.T) {
	dir := t.TempDir()
	cpuinfo := writeTempFile(t, dir, "cpuinfo", "Serial\t\t: 0000000000000000\n")

	if got := armSerialFrom(cpuinfo); got != "" {
		t.Errorf("[ERROR] [agent] [system] all-zero serial must be rejected, got %q", got)
	}
}

func TestARMSerialAbsent(t *testing.T) {
	dir := t.TempDir()
	cpuinfo := writeTempFile(t, dir, "cpuinfo", "processor\t: 0\nmodel name\t: Intel Xeon\n")

	if got := armSerialFrom(cpuinfo); got != "" {
		t.Errorf("[ERROR] [agent] [system] expected empty without Serial line, got %q", got)
	}
}
