//go:build windows

package webauthn

import (
	"errors"
	"syscall"
)

const processQueryLimitedInformation = 0x1000
const windowsProcessStillActive = 259

var (
	windowsOpenProcess        = syscall.OpenProcess
	windowsGetExitCodeProcess = syscall.GetExitCodeProcess
	windowsCloseHandle        = syscall.CloseHandle
)

func isProcessRunning(pid int) bool {
	windowsPID, ok := normalizeWindowsProcessID(pid)
	if !ok {
		return false
	}

	handle, err := windowsOpenProcess(processQueryLimitedInformation, false, windowsPID)
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}

	defer windowsCloseHandle(handle)

	var exitCode uint32
	if err := windowsGetExitCodeProcess(handle, &exitCode); err != nil {
		return true
	}

	return exitCode == windowsProcessStillActive
}
