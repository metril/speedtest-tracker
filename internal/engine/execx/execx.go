// Package execx runs external commands in their own process group so that
// cancelling the command's context reliably kills the whole group,
// including any grandchildren the command spawned in the background.
package execx

import (
	"context"
	"os/exec"
	"time"
)

// Command returns an *exec.Cmd for bin with args. It is configured so that
// cancelling ctx kills the entire process group, not just the direct
// child, and so Wait allows a short grace period for the process to exit
// on its own before its I/O pipes are forcibly closed.
func Command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = sysProcAttr()
	cmd.Cancel = func() error { return cancel(cmd) }
	cmd.WaitDelay = 2 * time.Second
	return cmd
}
