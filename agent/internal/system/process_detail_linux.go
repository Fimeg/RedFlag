//go:build linux
// +build linux

package system

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// getFullProcessSnapshot reads /proc to build a complete process inventory.
// No subprocess spawns — pure /proc reads following the pattern of process_linux.go.
func getFullProcessSnapshot() (*FullProcessSnapshot, error) {
	start := time.Now()

	totalCPU, err := readTotalCPUTicks()
	if err != nil {
		return nil, fmt.Errorf("read total cpu ticks: %w", err)
	}
	if totalCPU == 0 {
		return nil, fmt.Errorf("total CPU ticks is zero")
	}

	memTotal, err := readMemTotal()
	if err != nil {
		return nil, fmt.Errorf("read mem total: %w", err)
	}
	if memTotal == 0 {
		return nil, fmt.Errorf("mem total is zero")
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	// Pre-load /etc/passwd and /etc/group for uid→name resolution.
	passwd := readPasswd()
	group := readGroup()

	var procs []FullProcess
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		proc, err := readFullProc(pid, totalCPU, memTotal, passwd, group)
		if err != nil {
			continue // vanished or permission denied
		}
		procs = append(procs, *proc)
	}

	return &FullProcessSnapshot{
		Processes:    procs,
		ProcessCount: len(procs),
		ScannedAt:    start.UTC(),
		DurationMs:   time.Since(start).Milliseconds(),
	}, nil
}

// readFullProc reads all /proc/[pid]/* files for a single process.
func readFullProc(pid int, totalCPU, memTotal uint64, passwd, group map[uint32]string) (*FullProcess, error) {
	// /proc/[pid]/stat — core fields
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}

	proc := &FullProcess{PID: pid, OnDisk: -1}

	// Parse comm: between first '(' and last ')'
	firstParen := bytes.IndexByte(statData, '(')
	lastParen := bytes.LastIndexByte(statData, ')')
	if firstParen < 0 || lastParen <= firstParen {
		return nil, fmt.Errorf("malformed stat")
	}
	proc.Name = string(statData[firstParen+1 : lastParen])

	// Fields after ')'
	afterName := bytes.TrimSpace(statData[lastParen+1:])
	fields := strings.Fields(string(afterName))
	// Need at least 20 fields (state through nice)
	if len(fields) < 20 {
		return nil, fmt.Errorf("not enough fields in stat")
	}

	// /proc/[pid]/stat fields after "pid (comm)" — 0-indexed in fields[]:
	// [0]=state [1]=ppid [2]=pgrp [3]=session [4]=tty_nr [5]=tpgid [6]=flags
	// [7..10]=minflt..cmajflt [11]=utime [12]=stime [13]=cutime [14]=cstime
	// [15]=priority [16]=nice [17]=num_threads [18]=itrealvalue [19]=starttime
	proc.State = fields[0]
	proc.ParentPID = atoi(fields[1])
	proc.ProcessGroupID = atoi(fields[2])
	proc.TTY = atoi(fields[4])
	utime := atouint64(fields[11])
	stime := atouint64(fields[12])
	proc.Nice = atoi(fields[16])
	proc.StartTimeSeconds = atouint64(fields[19])

	// CPU seconds
	if totalCPU > 0 {
		proc.CPUSecondsUser = float64(utime) / float64(totalCPU) * 100
		proc.CPUSecondsSystem = float64(stime) / float64(totalCPU) * 100
		proc.CPUPercent = (float64(utime+stime) / float64(totalCPU)) * 100.0
	}

	// /proc/[pid]/status — memory, threads, uid/gid
	statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err == nil {
		for _, line := range bytes.Split(statusData, []byte("\n")) {
			switch {
			case bytes.HasPrefix(line, []byte("Uid:")):
				// Uid:\t<real>\t<effective>\t<saved>\t<fs>
				parts := strings.Fields(string(line))
				if len(parts) >= 3 {
					proc.UID = uint32(atoi(parts[1]))
					proc.EUID = uint32(atoi(parts[2]))
				}
			case bytes.HasPrefix(line, []byte("Gid:")):
				parts := strings.Fields(string(line))
				if len(parts) >= 3 {
					proc.GID = uint32(atoi(parts[1]))
					proc.EGID = uint32(atoi(parts[2]))
				}
			case bytes.HasPrefix(line, []byte("VmRSS:")):
				parts := strings.Fields(string(line))
				if len(parts) >= 2 {
					proc.RSSBytes = atouint64(parts[1]) * 1024 // KB to bytes
				}
			case bytes.HasPrefix(line, []byte("VmSize:")):
				parts := strings.Fields(string(line))
				if len(parts) >= 2 {
					proc.VMSBytes = atouint64(parts[1]) * 1024
				}
			case bytes.HasPrefix(line, []byte("Threads:")):
				parts := strings.Fields(string(line))
				if len(parts) >= 2 {
					proc.Threads = atoi(parts[1])
				}
			case bytes.HasPrefix(line, []byte("CapEff:")):
				parts := strings.Fields(string(line))
				if len(parts) >= 2 {
					proc.CapabilitiesEffective = parts[1]
				}
			}
		}
	}

	// Memory percent
	if memTotal > 0 {
		proc.MemPercent = (float64(proc.RSSBytes) / float64(memTotal*1024)) * 100.0
	}

	// Resolved user/group names
	proc.User = passwd[proc.UID]
	proc.Group = group[proc.GID]

	// Elevation
	if proc.UID != proc.EUID || proc.GID != proc.EGID {
		proc.ElevationStatus = "elevated"
	}

	// /proc/[pid]/exe — binary path
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	if link, err := os.Readlink(exePath); err == nil {
		proc.Path = link
		// On-disk check: does the binary exist at the resolved path?
		if _, err := os.Stat(link); err == nil {
			proc.OnDisk = 1
		} else {
			proc.OnDisk = 0
		}
	}

	// /proc/[pid]/cgroup — the owning unit or container
	if cgroupData, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid)); err == nil {
		proc.ProcessOwner = parseCgroupOwner(string(cgroupData))
	}

	// /proc/[pid]/cmdline — full command line
	cmdlineData, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err == nil {
		// NUL-delimited, join with spaces, trim trailing NUL
		proc.Cmdline = strings.ReplaceAll(strings.TrimRight(string(cmdlineData), "\x00"), "\x00", " ")
	}

	// /proc/[pid]/cwd — working directory
	cwdPath := fmt.Sprintf("/proc/%d/cwd", pid)
	if link, err := os.Readlink(cwdPath); err == nil {
		proc.Cwd = link
	}

	// TTY name resolution
	if proc.TTY > 0 {
		proc.TTYName = resolveTTY(proc.TTY)
	}

	// /proc/[pid]/io — disk I/O (may fail for other users' processes)
	ioData, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid))
	if err == nil {
		for _, line := range bytes.Split(ioData, []byte("\n")) {
			switch {
			case bytes.HasPrefix(line, []byte("read_bytes:")):
				proc.DiskBytesRead = atouint64(strings.TrimSpace(strings.TrimPrefix(string(line), "read_bytes:")))
			case bytes.HasPrefix(line, []byte("write_bytes:")):
				proc.DiskBytesWritten = atouint64(strings.TrimSpace(strings.TrimPrefix(string(line), "write_bytes:")))
			}
		}
	}

	return proc, nil
}

// readPasswd parses /etc/passwd into a uid→username map.
func readPasswd() map[uint32]string {
	m := make(map[uint32]string)
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return m
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		parts := strings.SplitN(string(line), ":", 7)
		if len(parts) >= 3 {
			if uid, err := strconv.ParseUint(parts[2], 10, 32); err == nil {
				m[uint32(uid)] = parts[0]
			}
		}
	}
	return m
}

// readGroup parses /etc/group into a gid→groupname map.
func readGroup() map[uint32]string {
	m := make(map[uint32]string)
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return m
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		parts := strings.SplitN(string(line), ":", 4)
		if len(parts) >= 3 {
			if gid, err := strconv.ParseUint(parts[2], 10, 32); err == nil {
				m[uint32(gid)] = parts[0]
			}
		}
	}
	return m
}

// resolveTTY maps a TTY device number (from /proc/[pid]/stat field 7) to /dev/pts/N or /dev/ttyN.
func resolveTTY(tty int) string {
	major := (tty >> 8) & 0xfff
	minor := (tty & 0xff) | ((tty >> 12) & 0xfff00)

	// /dev/pts/N — minor is the pts number
	if major == 136 || major == 188 {
		return fmt.Sprintf("pts/%d", minor)
	}
	// /dev/ttyN — major 4
	if major == 4 {
		if minor == 0 {
			return "tty0"
		}
		return fmt.Sprintf("tty%d", minor)
	}
	// /dev/tty (controlling terminal)
	if major == 5 && minor == 0 {
		return "tty"
	}
	return fmt.Sprintf("%d:%d", major, minor)
}

// getProcessDetail reads the related data (open files, sockets, env, etc.) for a
// single process. Called on drill-down, not on the initial list scan.
func getProcessDetail(pid int, caps ProcessCaps) (*FullProcess, error) {
	totalCPU, err := readTotalCPUTicks()
	if err != nil || totalCPU == 0 {
		return nil, fmt.Errorf("read total cpu ticks: %w", err)
	}
	memTotal, err := readMemTotal()
	if err != nil || memTotal == 0 {
		return nil, fmt.Errorf("read mem total: %w", err)
	}
	passwd := readPasswd()
	group := readGroup()

	proc, err := readFullProc(pid, totalCPU, memTotal, passwd, group)
	if err != nil {
		return nil, err
	}
	if owner, err := FindSoftwareOwner(proc.Path); err == nil {
		proc.PackageManager = owner.PackageType
		proc.PackageName = owner.PackageName
	}

	pidStr := strconv.Itoa(pid)

	// Single /proc/[pid]/fd/ walk for open files, pipe inodes, and socket inodes
	fdFiles, pipeInodes, socketInodes := walkProcFD(pidStr, caps.MaxOpenFiles, caps.MaxPipes)
	proc.OpenFiles = fdFiles
	proc.OpenPipes = pipeInodes

	// Sockets from /proc/[pid]/net/tcp, /proc/[pid]/net/tcp6, /proc/[pid]/net/unix
	proc.OpenSockets = readProcSockets(pidStr, caps.MaxSockets)

	// Environment variable names from /proc/[pid]/environ (keys only, no values)
	proc.EnvironmentVars = readEnvKeys(pidStr, caps.MaxEnvKeys)

	// Memory map from /proc/[pid]/maps
	proc.MemoryMap = readMemoryMap(pidStr, caps.MaxMemoryMap)

	// Namespaces from /proc/[pid]/ns/
	proc.Namespaces = readNamespaces(pidStr, caps.MaxNamespaces)

	// Effective capabilities, expanded from the mask the list scan already read
	proc.Capabilities = decodeCapabilities(proc.CapabilitiesEffective)

	// Listening ports: correlate socket inodes from fd walk with /proc/net/tcp
	proc.ListeningPorts = readListeningPorts(pid, socketInodes, caps.MaxListeningPorts)

	return proc, nil
}

// walkProcFD reads /proc/[pid]/fd/ once and partitions results into open files,
// pipe inodes, and socket inodes — avoiding three separate traversals.
func walkProcFD(pidStr string, maxFiles, maxPipes int) ([]ProcessOpenFile, []ProcessOpenPipe, map[uint64]bool) {
	dir := fmt.Sprintf("/proc/%s/fd", pidStr)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil
	}

	var files []ProcessOpenFile
	var pipes []ProcessOpenPipe
	socketInodes := make(map[uint64]bool)

	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		target, err := os.Readlink(fmt.Sprintf("%s/%d", dir, fd))
		if err != nil {
			continue
		}

		// Always collect socket inodes (needed for listening port correlation)
		// regardless of the file cap.
		if strings.HasPrefix(target, "socket:[") {
			inodeStr := strings.TrimPrefix(target, "socket:[")
			inodeStr = strings.TrimSuffix(inodeStr, "]")
			if inode := atouint64(inodeStr); inode > 0 {
				socketInodes[inode] = true
			}
		}

		// Cap files and pipes independently
		switch {
		case strings.HasPrefix(target, "socket:["):
			if maxFiles <= 0 || len(files) < maxFiles {
				files = append(files, ProcessOpenFile{FD: fd, Path: target, Type: "socket"})
			}
		case strings.HasPrefix(target, "pipe:["):
			if maxPipes <= 0 || len(pipes) < maxPipes {
				inodeStr := strings.TrimPrefix(target, "pipe:[")
				inodeStr = strings.TrimSuffix(inodeStr, "]")
				pipes = append(pipes, ProcessOpenPipe{FD: fd, Inode: atouint64(inodeStr)})
			}
			if maxFiles <= 0 || len(files) < maxFiles {
				files = append(files, ProcessOpenFile{FD: fd, Path: target, Type: "pipe"})
			}
		case strings.HasPrefix(target, "anon_inode:"):
			if maxFiles <= 0 || len(files) < maxFiles {
				files = append(files, ProcessOpenFile{FD: fd, Path: target, Type: "anon_inode"})
			}
		default:
			if maxFiles <= 0 || len(files) < maxFiles {
				files = append(files, ProcessOpenFile{FD: fd, Path: target, Type: "file"})
			}
		}
	}
	return files, pipes, socketInodes
}

// readProcSockets reads /proc/[pid]/net/tcp, tcp6, and unix for socket info.
func readProcSockets(pidStr string, maxSockets int) []ProcessOpenSocket {
	var sockets []ProcessOpenSocket

	// TCP sockets
	for _, proto := range []struct {
		file   string
		family string
		prot   string
	}{
		{fmt.Sprintf("/proc/%s/net/tcp", pidStr), "IPv4", "TCP"},
		{fmt.Sprintf("/proc/%s/net/tcp6", pidStr), "IPv6", "TCP"},
	} {
		if maxSockets > 0 && len(sockets) >= maxSockets {
			break
		}
		data, err := os.ReadFile(proto.file)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if maxSockets > 0 && len(sockets) >= maxSockets {
				break
			}
			if i == 0 || strings.TrimSpace(line) == "" {
				continue // skip header
			}
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			localAddr, localPort := parseHexAddr(fields[1])
			remoteAddr, remotePort := parseHexAddr(fields[2])
			state := tcpState(fields[3])
			inode := atouint64(fields[9])

			sockets = append(sockets, ProcessOpenSocket{
				Family:     proto.family,
				Protocol:   proto.prot,
				LocalAddr:  localAddr,
				LocalPort:  localPort,
				RemoteAddr: remoteAddr,
				RemotePort: remotePort,
				State:      state,
				Inode:      inode,
			})
		}
	}

	// UNIX sockets
	if maxSockets <= 0 || len(sockets) < maxSockets {
		unixFile := fmt.Sprintf("/proc/%s/net/unix", pidStr)
		data, err := os.ReadFile(unixFile)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if maxSockets > 0 && len(sockets) >= maxSockets {
					break
				}
				if i == 0 || strings.TrimSpace(line) == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 7 {
					continue
				}
				inode := atouint64(fields[6])
				path := ""
				if len(fields) > 7 {
					path = fields[7]
				}
				sockets = append(sockets, ProcessOpenSocket{
					Family:   "UNIX",
					Protocol: "UNIX",
					Inode:    inode,
					Path:     path,
				})
			}
		}
	}

	return sockets
}

// readEnvKeys reads /proc/[pid]/environ and returns only the key names.
// Values are never transmitted (security: env vars may contain secrets).
func readEnvKeys(pidStr string, maxKeys int) []string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%s/environ", pidStr))
	if err != nil {
		return nil
	}

	var keys []string
	for _, part := range bytes.Split(data, []byte{0}) {
		if maxKeys > 0 && len(keys) >= maxKeys {
			break
		}
		s := string(part)
		if idx := strings.IndexByte(s, '='); idx > 0 {
			keys = append(keys, s[:idx])
		}
	}
	return keys
}

// readMemoryMap reads /proc/[pid]/maps for memory-mapped regions.
func readMemoryMap(pidStr string, maxEntries int) []ProcessMemoryMap {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%s/maps", pidStr))
	if err != nil {
		return nil
	}

	var regions []ProcessMemoryMap
	for _, line := range strings.Split(string(data), "\n") {
		if maxEntries > 0 && len(regions) >= maxEntries {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Address range: start-end
		addrs := strings.SplitN(fields[0], "-", 2)
		if len(addrs) != 2 {
			continue
		}
		start, _ := strconv.ParseUint(addrs[0], 16, 64)
		end, _ := strconv.ParseUint(addrs[1], 16, 64)

		offset, _ := strconv.ParseUint(fields[2], 16, 64)
		inode := atouint64(fields[4])

		path := ""
		if len(fields) > 5 {
			path = strings.Join(fields[5:], " ")
		}

		regions = append(regions, ProcessMemoryMap{
			Start:       start,
			End:         end,
			Permissions: fields[1],
			Offset:      offset,
			Device:      fields[3],
			Inode:       inode,
			Path:        path,
		})
	}
	return regions
}

// readNamespaces reads /proc/[pid]/ns/ symlinks for namespace info.
func readNamespaces(pidStr string, maxNS int) []ProcessNamespace {
	dir := fmt.Sprintf("/proc/%s/ns", pidStr)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var ns []ProcessNamespace
	for _, entry := range entries {
		if maxNS > 0 && len(ns) >= maxNS {
			break
		}
		link := fmt.Sprintf("%s/%s", dir, entry.Name())
		target, err := os.Readlink(link)
		if err != nil {
			continue
		}
		// Target format: "type:[inode]" e.g. "cgroup:[4026531835]"
		inode := ""
		if idx := strings.Index(target, "["); idx >= 0 {
			inode = strings.TrimSuffix(target[idx+1:], "]")
		}
		ns = append(ns, ProcessNamespace{
			Type:  entry.Name(),
			Inode: inode,
		})
	}
	return ns
}

// readListeningPorts finds TCP listening ports that belong to a specific process
// by correlating socket inodes (from walkProcFD) with /proc/net/tcp entries.
func readListeningPorts(pid int, socketInodes map[uint64]bool, maxPorts int) []ProcessListeningPort {
	if len(socketInodes) == 0 {
		return nil
	}

	// Read system-wide /proc/net/tcp and /proc/net/tcp6, keep only LISTEN
	// entries whose inode matches one of this process's sockets.
	var ports []ProcessListeningPort
	for _, file := range []struct {
		path string
	}{
		{"/proc/net/tcp"},
		{"/proc/net/tcp6"},
	} {
		if maxPorts > 0 && len(ports) >= maxPorts {
			break
		}
		data, err := os.ReadFile(file.path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if maxPorts > 0 && len(ports) >= maxPorts {
				break
			}
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			if fields[3] != "0A" { // LISTEN only
				continue
			}
			inode := atouint64(fields[9])
			if !socketInodes[inode] {
				continue // not this process
			}
			localAddr, localPort := parseHexAddr(fields[1])
			ports = append(ports, ProcessListeningPort{
				Protocol:  "TCP",
				LocalAddr: localAddr,
				LocalPort: localPort,
				Socket:    inode,
			})
		}
	}
	return ports
}

// parseHexAddr parses "HEXADDR:HEXPORT" from /proc/net/tcp format.
// e.g. "0100007F:0050" → "127.0.0.1", 80
func parseHexAddr(hex string) (string, int) {
	parts := strings.SplitN(hex, ":", 2)
	if len(parts) != 2 {
		return "", 0
	}

	port, _ := strconv.ParseUint(parts[1], 16, 16)

	// Parse IP (little-endian hex for IPv4)
	addrHex := parts[0]
	if len(addrHex) == 8 {
		// IPv4: 4 bytes in little-endian
		b0, _ := strconv.ParseUint(addrHex[6:8], 16, 8)
		b1, _ := strconv.ParseUint(addrHex[4:6], 16, 8)
		b2, _ := strconv.ParseUint(addrHex[2:4], 16, 8)
		b3, _ := strconv.ParseUint(addrHex[0:2], 16, 8)
		return fmt.Sprintf("%d.%d.%d.%d", b0, b1, b2, b3), int(port)
	}
	if len(addrHex) == 32 {
		// IPv6: kernel stores as 4 little-endian 32-bit words (%08X%08X%08X%08X).
		// Reverse bytes within each 4-byte group to get standard IPv6 byte order.
		var ipv6 [16]byte
		for g := 0; g < 4; g++ {
			off := g * 8
			for j := 0; j < 4; j++ {
				b, _ := strconv.ParseUint(addrHex[off+j*2:off+j*2+2], 16, 8)
				ipv6[g*4+(3-j)] = byte(b)
			}
		}
		return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
			ipv6[0], ipv6[1], ipv6[2], ipv6[3], ipv6[4], ipv6[5], ipv6[6], ipv6[7],
			ipv6[8], ipv6[9], ipv6[10], ipv6[11], ipv6[12], ipv6[13], ipv6[14], ipv6[15]), int(port)
	}
	return addrHex, int(port)
}

// tcpState maps the hex state field from /proc/net/tcp to a human-readable name.
func tcpState(hex string) string {
	switch strings.ToUpper(hex) {
	case "01":
		return "ESTABLISHED"
	case "02":
		return "SYN_SENT"
	case "03":
		return "SYN_RECV"
	case "04":
		return "FIN_WAIT1"
	case "05":
		return "FIN_WAIT2"
	case "06":
		return "TIME_WAIT"
	case "07":
		return "CLOSE"
	case "08":
		return "CLOSE_WAIT"
	case "09":
		return "LAST_ACK"
	case "0A":
		return "LISTEN"
	case "0B":
		return "CLOSING"
	default:
		return hex
	}
}

// Helpers — avoid importing strconv for every tiny conversion.

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func atouint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// Ensure log is used (ETHOS: no silent imports).
var _ = log.Printf
