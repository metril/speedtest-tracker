//go:build unix

package ookla

import "syscall"

// sysProcAttr puts the CLI in its own process group so cancelling the
// context can kill the whole group.
func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
