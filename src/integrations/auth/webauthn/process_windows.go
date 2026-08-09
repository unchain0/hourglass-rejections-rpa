//go:build windows

package webauthn

import (
	"errors"
	"syscall"
)

const processQueryLimitedInformation = 0x1000

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}

	defer syscall.CloseHandle(handle)

	return true
}
