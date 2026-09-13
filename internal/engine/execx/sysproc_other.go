//go:build !unix

package execx

import (
	"os/exec"
	"syscall"
)

func sysProcAttr() *syscall.SysProcAttr { return nil }

// cancel kills just the direct child; non-unix platforms have no portable
// process-group kill in the standard library.
func cancel(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
