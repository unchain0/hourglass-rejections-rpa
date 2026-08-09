//go:build windows

package webauthn

import (
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsProcessRunningOnWindows(t *testing.T) {
	originalOpen := windowsOpenProcess
	originalExitCode := windowsGetExitCodeProcess
	originalClose := windowsCloseHandle
	t.Cleanup(func() {
		windowsOpenProcess = originalOpen
		windowsGetExitCodeProcess = originalExitCode
		windowsCloseHandle = originalClose
	})

	tests := []struct {
		name      string
		openErr   error
		exitCode  uint32
		exitErr   error
		wantAlive bool
	}{
		{name: "active", exitCode: windowsProcessStillActive, wantAlive: true},
		{name: "terminated", exitCode: 0},
		{name: "access denied", openErr: syscall.ERROR_ACCESS_DENIED, wantAlive: true},
		{name: "missing process", openErr: syscall.Errno(87)},
		{name: "unknown exit state", exitErr: errors.New("query failed"), wantAlive: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			closed := false
			windowsOpenProcess = func(uint32, bool, uint32) (syscall.Handle, error) {
				return syscall.Handle(1), test.openErr
			}
			windowsGetExitCodeProcess = func(_ syscall.Handle, got *uint32) error {
				*got = test.exitCode
				return test.exitErr
			}
			windowsCloseHandle = func(syscall.Handle) error {
				closed = true
				return nil
			}

			assert.Equal(t, test.wantAlive, isProcessRunning(123))
			assert.Equal(t, test.openErr == nil, closed)
		})
	}
}
