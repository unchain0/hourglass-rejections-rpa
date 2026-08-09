package webauthn

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeWindowsProcessID(t *testing.T) {
	type testCase struct {
		name string
		pid  int
		want uint32
		ok   bool
	}

	tests := []testCase{
		{name: "negative", pid: -1},
		{name: "zero", pid: 0},
		{name: "minimum valid", pid: 1, want: 1, ok: true},
	}

	if strconv.IntSize == 64 {
		tests = append(tests,
			testCase{name: "maximum valid", pid: int(math.MaxUint32), want: math.MaxUint32, ok: true},
			testCase{name: "above maximum", pid: int(uint64(math.MaxUint32) + 1)},
		)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := normalizeWindowsProcessID(test.pid)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.want, got)
		})
	}
}
