package system

import (
	"errors"
	"sync"
	"time"
)

var ErrMonitorUnavailable = errors.New("system resource monitor unavailable")

// ResourceMonitor owns the bounded, live machine history consumed by Desktop.
// Collection stays in the Agent: UIs receive observations and express intent;
// they never learn a second way to inspect or mutate the machine.
type ResourceMonitor struct {
	mu       sync.RWMutex
	interval time.Duration
	capacity int
	stop     chan struct{}
	done     chan struct{}
	started  bool
	previous *rawMonitorSample
	latest   *ResourceSnapshot
	lastErr  error
	history  []ResourcePoint
}

type ResourceSnapshot struct {
	CollectedAt time.Time        `json:"collected_at"`
	IntervalMS  int64            `json:"interval_ms"`
	CPU         CPUMetrics       `json:"cpu"`
	Memory      MemoryMetrics    `json:"memory"`
	Network     NetworkMetrics   `json:"network"`
	Storage     StorageMetrics   `json:"storage"`
	Thermals    []ThermalReading `json:"thermals,omitempty"`
	History     []ResourcePoint  `json:"history"`
}

type CPUMetrics struct {
	UsagePercent  float64     `json:"usage_percent"`
	UserPercent   float64     `json:"user_percent"`
	SystemPercent float64     `json:"system_percent"`
	IOWaitPercent float64     `json:"iowait_percent"`
	Load1         float64     `json:"load_1"`
	Load5         float64     `json:"load_5"`
	Load15        float64     `json:"load_15"`
	PerCore       []CoreUsage `json:"per_core"`
}

type CoreUsage struct {
	Core         int     `json:"core"`
	UsagePercent float64 `json:"usage_percent"`
}

type MemoryMetrics struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	SwapPercent    float64 `json:"swap_percent"`
}

type NetworkMetrics struct {
	ReceiveBytesPerSecond  float64            `json:"receive_bytes_per_second"`
	TransmitBytesPerSecond float64            `json:"transmit_bytes_per_second"`
	ReceiveBytes           uint64             `json:"receive_bytes"`
	TransmitBytes          uint64             `json:"transmit_bytes"`
	Interfaces             []NetworkInterface `json:"interfaces"`
}

type NetworkInterface struct {
	Name                   string  `json:"name"`
	ReceiveBytesPerSecond  float64 `json:"receive_bytes_per_second"`
	TransmitBytesPerSecond float64 `json:"transmit_bytes_per_second"`
	ReceiveBytes           uint64  `json:"receive_bytes"`
	TransmitBytes          uint64  `json:"transmit_bytes"`
}

type StorageMetrics struct {
	ReadBytesPerSecond  float64 `json:"read_bytes_per_second"`
	WriteBytesPerSecond float64 `json:"write_bytes_per_second"`
	ReadBytes           uint64  `json:"read_bytes"`
	WriteBytes          uint64  `json:"write_bytes"`
}

type ThermalReading struct {
	Name    string  `json:"name"`
	Celsius float64 `json:"celsius"`
}

type ResourcePoint struct {
	Timestamp             time.Time `json:"timestamp"`
	CPUPercent            float64   `json:"cpu_percent"`
	MemoryPercent         float64   `json:"memory_percent"`
	NetworkReceivePerSec  float64   `json:"network_receive_per_second"`
	NetworkTransmitPerSec float64   `json:"network_transmit_per_second"`
	DiskReadPerSec        float64   `json:"disk_read_per_second"`
	DiskWritePerSec       float64   `json:"disk_write_per_second"`
}

type cpuCounters struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

func (c cpuCounters) total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}

func (c cpuCounters) idle() uint64 { return c.Idle + c.IOWait }

type byteCounters struct{ Receive, Transmit uint64 }

type rawMonitorSample struct {
	at        time.Time
	cpu       cpuCounters
	cores     []cpuCounters
	load      [3]float64
	memory    MemoryMetrics
	network   map[string]byteCounters
	diskRead  uint64
	diskWrite uint64
	thermals  []ThermalReading
}

func NewResourceMonitor(interval time.Duration, capacity int) *ResourceMonitor {
	if interval <= 0 {
		interval = time.Second
	}
	if capacity <= 0 {
		capacity = 300
	}
	return &ResourceMonitor{
		interval: interval,
		capacity: capacity,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (m *ResourceMonitor) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	go func() {
		defer close(m.done)
		m.collect()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.collect()
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *ResourceMonitor) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	close(m.stop)
	m.mu.Unlock()
	<-m.done
}

func (m *ResourceMonitor) Snapshot() (*ResourceSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.latest == nil {
		if m.lastErr != nil {
			return nil, m.lastErr
		}
		return nil, ErrMonitorUnavailable
	}
	copy := *m.latest
	copy.CPU.PerCore = append([]CoreUsage(nil), m.latest.CPU.PerCore...)
	copy.Network.Interfaces = append([]NetworkInterface(nil), m.latest.Network.Interfaces...)
	copy.Thermals = append([]ThermalReading(nil), m.latest.Thermals...)
	copy.History = append([]ResourcePoint(nil), m.history...)
	return &copy, nil
}

func (m *ResourceMonitor) collect() {
	raw, err := readRawMonitorSample()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.lastErr = err
		return
	}

	snapshot := snapshotFromRaw(raw, m.previous, m.interval)
	m.previous = raw
	m.lastErr = nil
	point := ResourcePoint{
		Timestamp:             snapshot.CollectedAt,
		CPUPercent:            snapshot.CPU.UsagePercent,
		MemoryPercent:         snapshot.Memory.UsedPercent,
		NetworkReceivePerSec:  snapshot.Network.ReceiveBytesPerSecond,
		NetworkTransmitPerSec: snapshot.Network.TransmitBytesPerSecond,
		DiskReadPerSec:        snapshot.Storage.ReadBytesPerSecond,
		DiskWritePerSec:       snapshot.Storage.WriteBytesPerSecond,
	}
	m.history = append(m.history, point)
	if len(m.history) > m.capacity {
		copy(m.history, m.history[len(m.history)-m.capacity:])
		m.history = m.history[:m.capacity]
	}
	snapshot.History = append([]ResourcePoint(nil), m.history...)
	m.latest = snapshot
}

func snapshotFromRaw(now, previous *rawMonitorSample, fallback time.Duration) *ResourceSnapshot {
	elapsed := fallback.Seconds()
	if previous != nil {
		elapsed = now.at.Sub(previous.at).Seconds()
	}
	if elapsed <= 0 {
		elapsed = 1
	}

	cpu := CPUMetrics{Load1: now.load[0], Load5: now.load[1], Load15: now.load[2]}
	if previous != nil {
		cpu.UsagePercent, cpu.UserPercent, cpu.SystemPercent, cpu.IOWaitPercent = cpuDelta(now.cpu, previous.cpu)
		for index, counters := range now.cores {
			usage := 0.0
			if index < len(previous.cores) {
				usage, _, _, _ = cpuDelta(counters, previous.cores[index])
			}
			cpu.PerCore = append(cpu.PerCore, CoreUsage{Core: index, UsagePercent: usage})
		}
	}

	network := NetworkMetrics{}
	for name, counters := range now.network {
		iface := NetworkInterface{Name: name, ReceiveBytes: counters.Receive, TransmitBytes: counters.Transmit}
		if previous != nil {
			before := previous.network[name]
			iface.ReceiveBytesPerSecond = perSecond(counters.Receive, before.Receive, elapsed)
			iface.TransmitBytesPerSecond = perSecond(counters.Transmit, before.Transmit, elapsed)
		}
		network.Interfaces = append(network.Interfaces, iface)
		if name != "lo" {
			network.ReceiveBytes += counters.Receive
			network.TransmitBytes += counters.Transmit
			network.ReceiveBytesPerSecond += iface.ReceiveBytesPerSecond
			network.TransmitBytesPerSecond += iface.TransmitBytesPerSecond
		}
	}

	storage := StorageMetrics{ReadBytes: now.diskRead, WriteBytes: now.diskWrite}
	if previous != nil {
		storage.ReadBytesPerSecond = perSecond(now.diskRead, previous.diskRead, elapsed)
		storage.WriteBytesPerSecond = perSecond(now.diskWrite, previous.diskWrite, elapsed)
	}

	return &ResourceSnapshot{
		CollectedAt: now.at,
		IntervalMS:  int64(elapsed * 1000),
		CPU:         cpu,
		Memory:      now.memory,
		Network:     network,
		Storage:     storage,
		Thermals:    now.thermals,
	}
}

func cpuDelta(now, before cpuCounters) (usage, user, system, iowait float64) {
	total := now.total() - before.total()
	if total == 0 {
		return 0, 0, 0, 0
	}
	percent := func(delta uint64) float64 { return float64(delta) * 100 / float64(total) }
	return percent(total - (now.idle() - before.idle())),
		percent((now.User + now.Nice) - (before.User + before.Nice)),
		percent((now.System + now.IRQ + now.SoftIRQ) - (before.System + before.IRQ + before.SoftIRQ)),
		percent(now.IOWait - before.IOWait)
}

func perSecond(now, before uint64, elapsed float64) float64 {
	if now < before || elapsed <= 0 {
		return 0
	}
	return float64(now-before) / elapsed
}
