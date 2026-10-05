//go:build unix

package execx

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// sysProcAttr puts the child in its own process group so the whole group
// can be killed on cancel.
func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

// cancel kills the child's entire process group, reaching grandchildren it
// spawned (e.g. into the background) that a plain process kill would miss.
func cancel(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		// Group already gone (child exited between Wait and cancel).
		return os.ErrProcessDone
	}
	return err
}
