package webauthn

import "math"

func normalizeWindowsProcessID(pid int) (uint32, bool) {
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return 0, false
	}

	return uint32(pid), true
}
