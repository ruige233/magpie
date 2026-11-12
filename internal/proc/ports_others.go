//go:build !darwin && !linux

package proc

import "context"

// portsOf can't ask this system what a process listens on: no port rather
// than a wrong one.
func portsOf(context.Context, int) []int { return nil }
