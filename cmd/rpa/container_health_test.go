package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerHealthCheckUsesDaemonLiveness(t *testing.T) {
	dockerfile, err := os.ReadFile("../../Dockerfile")
	require.NoError(t, err)
	compose, err := os.ReadFile("../../docker-compose.yml")
	require.NoError(t, err)

	assert.Contains(t, string(dockerfile), `test "$(cat /proc/1/comm)" = "rpa"`)
	assert.NotContains(t, string(dockerfile), "pgrep -x rpa")
	assert.Contains(t, string(compose), "/proc/1/comm")
	assert.NotContains(t, string(compose), "pgrep -x rpa")
}
