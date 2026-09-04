package system

import (
	"math"
	"testing"
	"time"
)

func TestSnapshotFromRawComputesRatesAndHistoryPointInputs(t *testing.T) {
	before := &rawMonitorSample{
		at:        time.Unix(100, 0),
		cpu:       cpuCounters{User: 100, System: 50, Idle: 850},
		cores:     []cpuCounters{{User: 100, Idle: 900}},
		memory:    MemoryMetrics{TotalBytes: 1000, UsedBytes: 400, AvailableBytes: 600, UsedPercent: 40},
		network:   map[string]byteCounters{"eth0": {Receive: 1000, Transmit: 2000}},
		diskRead:  5000,
		diskWrite: 8000,
	}
	now := &rawMonitorSample{
		at:        time.Unix(102, 0),
		cpu:       cpuCounters{User: 180, System: 70, Idle: 950},
		cores:     []cpuCounters{{User: 160, Idle: 1040}},
		load:      [3]float64{1.2, 0.8, 0.5},
		memory:    MemoryMetrics{TotalBytes: 1000, UsedBytes: 450, AvailableBytes: 550, UsedPercent: 45},
		network:   map[string]byteCounters{"eth0": {Receive: 3000, Transmit: 5000}},
		diskRead:  9000,
		diskWrite: 14000,
	}

	snapshot := snapshotFromRaw(now, before, time.Second)
	assertNear(t, snapshot.CPU.UsagePercent, 50)
	assertNear(t, snapshot.CPU.UserPercent, 40)
	assertNear(t, snapshot.CPU.SystemPercent, 10)
	assertNear(t, snapshot.Network.ReceiveBytesPerSecond, 1000)
	assertNear(t, snapshot.Network.TransmitBytesPerSecond, 1500)
	assertNear(t, snapshot.Storage.ReadBytesPerSecond, 2000)
	assertNear(t, snapshot.Storage.WriteBytesPerSecond, 3000)
	if len(snapshot.CPU.PerCore) != 1 {
		t.Fatalf("per-core samples = %d, want 1", len(snapshot.CPU.PerCore))
	}
}

func TestResourceMonitorHistoryIsBounded(t *testing.T) {
	monitor := NewResourceMonitor(time.Second, 2)
	monitor.history = []ResourcePoint{{CPUPercent: 1}, {CPUPercent: 2}}
	monitor.capacity = 2
	monitor.latest = &ResourceSnapshot{}
	monitor.previous = nil

	monitor.mu.Lock()
	monitor.history = append(monitor.history, ResourcePoint{CPUPercent: 3})
	if len(monitor.history) > monitor.capacity {
		copy(monitor.history, monitor.history[len(monitor.history)-monitor.capacity:])
		monitor.history = monitor.history[:monitor.capacity]
	}
	monitor.mu.Unlock()

	if len(monitor.history) != 2 || monitor.history[0].CPUPercent != 2 || monitor.history[1].CPUPercent != 3 {
		t.Fatalf("bounded history = %#v", monitor.history)
	}
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("got %.3f, want %.3f", got, want)
	}
}
