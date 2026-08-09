//go:build !windows

package webauthn

import (
	"errors"
	"syscall"
)

func isProcessRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
