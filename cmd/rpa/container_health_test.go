package main

import (
	"os"
	"strings"
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

func TestDockerBuildLimitsPeakMemory(t *testing.T) {
	dockerfile, err := os.ReadFile("../../Dockerfile")
	require.NoError(t, err)
	contents := string(dockerfile)

	assert.Contains(t, contents, "GOMAXPROCS=1")
	assert.Contains(t, contents, "GOMEMLIMIT=320MiB")
	assert.Contains(t, contents, "go build -p=1")
	assert.Contains(t, contents, "--mount=type=cache,target=/go/pkg/mod")
	assert.Contains(t, contents, "--mount=type=cache,target=/root/.cache/go-build")
	assert.Contains(t, contents, "./cmd/rpa")
	assert.Contains(t, contents, "./cmd/token-refresh")
	assert.NotContains(t, contents, "./cmd/setup-auth")
	assert.NotContains(t, contents, "./cmd/save-tokens")
	assert.NotContains(t, contents, "./cmd/hourglass")
	assert.NotContains(t, contents, "chromium")
	assert.Contains(t, contents, "apk add --no-cache ca-certificates tzdata curl")

	pkgs := strings.Index(contents, "RUN apk add --no-cache ca-certificates tzdata curl")
	binaries := strings.Index(contents, "COPY --from=builder --chown=rpa:rpa /out/ ./")
	require.NotEqual(t, -1, pkgs)
	require.NotEqual(t, -1, binaries)
	assert.Less(t, pkgs, binaries)
}
