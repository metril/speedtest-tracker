//go:build !unix

package ookla

import "syscall"

func sysProcAttr() *syscall.SysProcAttr { return nil }
