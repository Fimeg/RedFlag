//go:build linux

package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func readRawMonitorSample() (*rawMonitorSample, error) {
	cpu, cores, err := readCPUCounters("/proc/stat")
	if err != nil {
		return nil, err
	}
	memory, err := readMemoryMetrics("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	load, _ := readLoadAverage("/proc/loadavg")
	network, _ := readNetworkCounters("/proc/net/dev")
	diskRead, diskWrite := readDiskCounters()
	return &rawMonitorSample{
		at:        time.Now().UTC(),
		cpu:       cpu,
		cores:     cores,
		load:      load,
		memory:    memory,
		network:   network,
		diskRead:  diskRead,
		diskWrite: diskWrite,
		thermals:  readThermals(),
	}, nil
}

func readCPUCounters(path string) (cpuCounters, []cpuCounters, error) {
	file, err := os.Open(path)
	if err != nil {
		return cpuCounters{}, nil, fmt.Errorf("read cpu counters: %w", err)
	}
	defer file.Close()
	var total cpuCounters
	var cores []cpuCounters
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			if len(cores) > 0 {
				break
			}
			continue
		}
		counters := parseCPUFields(fields[1:])
		if fields[0] == "cpu" {
			total = counters
		} else {
			cores = append(cores, counters)
		}
	}
	if err := scanner.Err(); err != nil {
		return cpuCounters{}, nil, err
	}
	if total.total() == 0 {
		return cpuCounters{}, nil, fmt.Errorf("cpu counters are empty")
	}
	return total, cores, nil
}

func parseCPUFields(fields []string) cpuCounters {
	values := make([]uint64, 8)
	for i := 0; i < len(values) && i < len(fields); i++ {
		values[i], _ = strconv.ParseUint(fields[i], 10, 64)
	}
	return cpuCounters{User: values[0], Nice: values[1], System: values[2], Idle: values[3], IOWait: values[4], IRQ: values[5], SoftIRQ: values[6], Steal: values[7]}
}

func readMemoryMetrics(path string) (MemoryMetrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MemoryMetrics{}, fmt.Errorf("read memory counters: %w", err)
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			values[key] = value * 1024
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return MemoryMetrics{}, fmt.Errorf("MemTotal is unavailable")
	}
	used := total - available
	swapTotal := values["SwapTotal"]
	swapUsed := swapTotal - values["SwapFree"]
	metrics := MemoryMetrics{TotalBytes: total, UsedBytes: used, AvailableBytes: available, UsedPercent: float64(used) * 100 / float64(total), SwapTotalBytes: swapTotal, SwapUsedBytes: swapUsed}
	if swapTotal > 0 {
		metrics.SwapPercent = float64(swapUsed) * 100 / float64(swapTotal)
	}
	return metrics, nil
}

func readLoadAverage(path string) ([3]float64, error) {
	var load [3]float64
	data, err := os.ReadFile(path)
	if err != nil {
		return load, err
	}
	fields := strings.Fields(string(data))
	for i := 0; i < 3 && i < len(fields); i++ {
		load[i], _ = strconv.ParseFloat(fields[i], 64)
	}
	return load, nil
}

func readNetworkCounters(path string) (map[string]byteCounters, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]byteCounters)
	for _, line := range strings.Split(string(data), "\n") {
		name, values, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 9 {
			continue
		}
		receive, _ := strconv.ParseUint(fields[0], 10, 64)
		transmit, _ := strconv.ParseUint(fields[8], 10, 64)
		result[strings.TrimSpace(name)] = byteCounters{Receive: receive, Transmit: transmit}
	}
	return result, nil
}

func readDiskCounters() (uint64, uint64) {
	devices, err := os.ReadDir("/sys/block")
	if err != nil {
		return 0, 0
	}
	whole := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		name := device.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
			continue
		}
		whole[name] = struct{}{}
	}
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return 0, 0
	}
	var read, written uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		if _, ok := whole[fields[2]]; !ok {
			continue
		}
		sectorsRead, _ := strconv.ParseUint(fields[5], 10, 64)
		sectorsWritten, _ := strconv.ParseUint(fields[9], 10, 64)
		read += sectorsRead * 512
		written += sectorsWritten * 512
	}
	return read, written
}

func readThermals() []ThermalReading {
	paths, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	var readings []ThermalReading
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		name := filepath.Base(filepath.Dir(path))
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "type")); err == nil {
			name = strings.TrimSpace(string(data))
		}
		if value > 1000 {
			value /= 1000
		}
		if value > -100 && value < 250 {
			readings = append(readings, ThermalReading{Name: name, Celsius: value})
		}
	}
	sort.Slice(readings, func(i, j int) bool { return readings[i].Name < readings[j].Name })
	return readings
}
