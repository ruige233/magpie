//go:build darwin || linux

package proc

import (
	"context"
	"os"
	"strconv"
)

// portsOf asks lsof which ports process pid listens on, -F n printing one
// "n<address>" per socket. A system without lsof, and another user's
// process, reads as no port at all.
func portsOf(ctx context.Context, pid int) []int {
	lsof := "/usr/sbin/lsof"
	if _, err := os.Stat(lsof); err != nil {
		lsof = "lsof"
	}
	out, err := CommandContext(ctx, lsof, "-nP", "-a", "-p", strconv.Itoa(pid), "-iTCP", "-sTCP:LISTEN", "-F", "n").Output()
	if err != nil {
		// lsof says 1 for none found
		return nil
	}
	return parsePorts(string(out))
}
