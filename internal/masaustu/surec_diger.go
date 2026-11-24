//go:build !windows

package masaustu

import "os/exec"

// penceresizHazirla yalnızca Windows'ta bir şey yapar (konsol penceresi).
func penceresizHazirla(*exec.Cmd) {}
