package system

import "testing"

func TestParseWindowsCPUInfoJSON(t *testing.T) {
	data := []byte(`{
		"Name": "Intel(R) Core(TM) i7-6700 CPU @ 3.40GHz",
		"NumberOfCores": 4,
		"NumberOfLogicalProcessors": 8
	}`)

	cpu, err := parseWindowsCPUInfoJSON(data)
	if err != nil {
		t.Fatalf("parseWindowsCPUInfoJSON() error = %v", err)
	}

	if cpu.ModelName != "Intel(R) Core(TM) i7-6700 CPU @ 3.40GHz" {
		t.Fatalf("ModelName = %q", cpu.ModelName)
	}
	if cpu.Cores != 4 {
		t.Fatalf("Cores = %d, want 4", cpu.Cores)
	}
	if cpu.Threads != 8 {
		t.Fatalf("Threads = %d, want 8", cpu.Threads)
	}
}

func TestParseWindowsCPUInfoJSONAcceptsStringNumbers(t *testing.T) {
	data := []byte(`{
		"Name": "AMD Ryzen",
		"NumberOfCores": "6",
		"NumberOfLogicalProcessors": "12"
	}`)

	cpu, err := parseWindowsCPUInfoJSON(data)
	if err != nil {
		t.Fatalf("parseWindowsCPUInfoJSON() error = %v", err)
	}

	if cpu.Cores != 6 || cpu.Threads != 12 {
		t.Fatalf("CPU counts = cores:%d threads:%d, want cores:6 threads:12", cpu.Cores, cpu.Threads)
	}
}

func TestParseWindowsCPUInfoJSONDefaultsThreadsToCores(t *testing.T) {
	data := []byte(`{
		"Name": "CPU",
		"NumberOfCores": 4,
		"NumberOfLogicalProcessors": null
	}`)

	cpu, err := parseWindowsCPUInfoJSON(data)
	if err != nil {
		t.Fatalf("parseWindowsCPUInfoJSON() error = %v", err)
	}

	if cpu.Threads != 4 {
		t.Fatalf("Threads = %d, want 4", cpu.Threads)
	}
}
