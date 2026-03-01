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

	assert.Equal(t, "https://app.hourglass-app.com/v2/page/app", cfg.HourglassURL)
	assert.Equal(t, "cookies.json", cfg.CookieFile)
	assert.Equal(t, "./outputs", cfg.OutputDir)
	assert.Equal(t, false, cfg.Debug)
	assert.Equal(t, "0 9 * * *", cfg.ScheduleMorning)
	assert.Equal(t, "0 17 * * *", cfg.ScheduleEvening)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
}

func TestLoad_Overrides(t *testing.T) {
	os.Setenv("HOURGLASS_URL", "https://test.com")
	os.Setenv("COOKIE_FILE", "test_cookies.json")
	os.Setenv("OUTPUT_DIR", "/tmp/test")
	os.Setenv("DEBUG", "true")
	os.Setenv("SCHEDULE_MORNING", "0 8 * * *")
	os.Setenv("SCHEDULE_EVENING", "0 18 * * *")
	os.Setenv("TIMEOUT", "30s")

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
}
