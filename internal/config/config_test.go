package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoad_Defaults(t *testing.T) {
	// Ensure environment is clean for this test
	os.Clearenv()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "https://app.hourglass-app.com", cfg.HourglassURL)
	assert.Equal(t, "cookies.json", cfg.CookieFile)
	assert.Equal(t, "./outputs", cfg.OutputDir)
	assert.Equal(t, false, cfg.Debug)
	assert.Equal(t, "0 9 * * *", cfg.ScheduleMorning)
	assert.Equal(t, "0 17 * * *", cfg.ScheduleEvening)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Equal(t, "production", cfg.SentryEnvironment)
	assert.Empty(t, cfg.HourglassXSRFToken)
	assert.Empty(t, cfg.HourglassHGLogin)
	assert.Empty(t, cfg.SentryDSN)
}
func TestLoad_Overrides(t *testing.T) {
	os.Setenv("HOURGLASS_URL", "https://test.com")
	os.Setenv("COOKIE_FILE", "test_cookies.json")
	os.Setenv("OUTPUT_DIR", "/tmp/test")
	os.Setenv("DEBUG", "true")
	os.Setenv("SCHEDULE_MORNING", "0 8 * * *")
	os.Setenv("SCHEDULE_EVENING", "0 18 * * *")
	os.Setenv("TIMEOUT", "30s")
	os.Setenv("SENTRY_ENVIRONMENT", "staging")
	os.Setenv("HOURGLASS_XSRF_TOKEN", "test-xsrf-token")
	os.Setenv("HOURGLASS_HGLOGIN_COOKIE", "test-hglogin-cookie")
	os.Setenv("SENTRY_DSN", "https://test-sentry-dsn")
	defer os.Clearenv()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "https://test.com", cfg.HourglassURL)
	assert.Equal(t, "test_cookies.json", cfg.CookieFile)
	assert.Equal(t, "/tmp/test", cfg.OutputDir)
	assert.Equal(t, true, cfg.Debug)
	assert.Equal(t, "0 8 * * *", cfg.ScheduleMorning)
	assert.Equal(t, "0 18 * * *", cfg.ScheduleEvening)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, "staging", cfg.SentryEnvironment)
	assert.Equal(t, "test-xsrf-token", cfg.HourglassXSRFToken)
	assert.Equal(t, "test-hglogin-cookie", cfg.HourglassHGLogin)
	assert.Equal(t, "https://test-sentry-dsn", cfg.SentryDSN)
}
func TestLoad_Error_InvalidDuration(t *testing.T) {
	os.Clearenv()
	os.Setenv("TIMEOUT", "invalid-duration")
	defer os.Clearenv()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Timeout")
}

func TestLoad_Error_InvalidBool(t *testing.T) {
	os.Clearenv()
	os.Setenv("DEBUG", "not-a-boolean")
	defer os.Clearenv()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Debug")
}

func TestLoad_Error_InvalidTimeoutNumber(t *testing.T) {
	os.Clearenv()
	os.Setenv("TIMEOUT", "abc")
	defer os.Clearenv()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Timeout")
}
