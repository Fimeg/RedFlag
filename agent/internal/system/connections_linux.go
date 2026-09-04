//go:build linux

package system

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func getConnectionsSnapshot() (*ConnectionSnapshot, error) {
	owners := socketOwners()
	var connections []Connection
	for _, source := range []struct {
		path, protocol, family string
	}{
		{"/proc/net/tcp", "TCP", "IPv4"},
		{"/proc/net/tcp6", "TCP", "IPv6"},
		{"/proc/net/udp", "UDP", "IPv4"},
		{"/proc/net/udp6", "UDP", "IPv6"},
	} {
		data, err := os.ReadFile(source.path)
		if err != nil {
			continue
		}
		for index, line := range strings.Split(string(data), "\n") {
			if index == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			localAddr, localPort := parseHexAddr(fields[1])
			remoteAddr, remotePort := parseHexAddr(fields[2])
			inode := atouint64(fields[9])
			owner := owners[inode]
			state := fields[3]
			if source.protocol == "TCP" {
				state = tcpState(state)
			}
			connections = append(connections, Connection{
				PID: owner.pid, Process: owner.name, Protocol: source.protocol, Family: source.family,
				LocalAddr: localAddr, LocalPort: localPort, RemoteAddr: remoteAddr,
				RemotePort: remotePort, State: state, Inode: inode,
			})
		}
	}
	if len(connections) == 0 {
		if _, err := os.Stat("/proc/net/tcp"); err != nil {
			return nil, fmt.Errorf("read network connection table: %w", err)
		}
	}
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].State != connections[j].State {
			return connections[i].State == "LISTEN"
		}
		if connections[i].Process != connections[j].Process {
			return connections[i].Process < connections[j].Process
		}
		return connections[i].LocalPort < connections[j].LocalPort
	})
	return &ConnectionSnapshot{Connections: connections, Count: len(connections), CollectedAt: time.Now().UTC()}, nil
}

type socketOwner struct {
	pid  int
	name string
}

func socketOwners() map[uint64]socketOwner {
	owners := make(map[uint64]socketOwner)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return owners
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		nameBytes, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		name := strings.TrimSpace(string(nameBytes))
		fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			value, err := strconv.ParseUint(inode, 10, 64)
			if err == nil {
				owners[value] = socketOwner{pid: pid, name: name}
			}
		}
	}
	return owners
}
