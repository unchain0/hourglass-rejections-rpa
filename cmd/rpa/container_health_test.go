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

	binaries := strings.Index(contents, "COPY --from=builder /out/ ./")
	chromium := strings.Index(contents, "RUN apk add --no-cache")
	require.NotEqual(t, -1, binaries)
	require.NotEqual(t, -1, chromium)
	assert.Less(t, binaries, chromium, "the runtime stage must wait for the builder before installing Chromium")
}
