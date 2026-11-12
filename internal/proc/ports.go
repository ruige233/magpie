package proc

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// PortsOf are the TCP ports process pid listens on, lowest first: nil when
// it listens on none, and where the system won't say what a process holds
// (Windows, and any system without lsof), where a caller shows no port
// rather than a wrong one.
func PortsOf(ctx context.Context, pid int) []int { return portsOf(ctx, pid) }

// parsePorts reads lsof's -F n output, a "n<address>" line per socket, the
// port after the last ':' ("n*:3081", "n127.0.0.1:3081", "n[::1]:3081").
func parsePorts(out string) []int {
	seen := map[int]bool{}
	var ports []int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		addr, ok := strings.CutPrefix(line, "n")
		if !ok {
			continue
		}
		i := strings.LastIndex(addr, ":")
		if i < 0 {
			continue
		}
		port, err := strconv.Atoi(addr[i+1:])
		if err != nil || port <= 0 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}
