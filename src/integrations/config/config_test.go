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
	assert.Equal(t, false, cfg.Debug)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "0 9 * * *", cfg.ScheduleMorning)
	assert.Equal(t, "0 17 * * *", cfg.ScheduleEvening)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Equal(t, 30*time.Second, cfg.OTLPMetricInterval)
	assert.Equal(t, "hourglass-rejections-rpa", cfg.TelemetryServiceName)
	assert.Equal(t, "1.0.0", cfg.TelemetryServiceVersion)
	assert.Equal(t, "production", cfg.DeploymentEnvironment)
	assert.Empty(t, cfg.HourglassXSRFToken)
	assert.Empty(t, cfg.HourglassHGLogin)
	assert.Empty(t, cfg.DatabaseURL)
	assert.Empty(t, cfg.OTLPEndpoint)
	assert.Empty(t, cfg.OTLPHeaders)
	assert.True(t, cfg.AutoRefreshTokens)
}
func TestLoad_Overrides(t *testing.T) {
	os.Setenv("HOURGLASS_URL", "https://test.com")
	os.Setenv("DEBUG", "true")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("SCHEDULE_MORNING", "0 8 * * *")
	os.Setenv("SCHEDULE_EVENING", "0 18 * * *")
	os.Setenv("TIMEOUT", "30s")
	os.Setenv("DEPLOYMENT_ENVIRONMENT", "staging")
	os.Setenv("HOURGLASS_XSRF_TOKEN", "test-xsrf-token")
	os.Setenv("HOURGLASS_HGLOGIN_COOKIE", "test-hglogin-cookie")
	os.Setenv("WEBAUTHN_CREDENTIALS_PATH", "/tmp/webauthn-credentials.json")
	os.Setenv("AUTO_REFRESH_TOKENS", "false")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/hourglass")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://otel.example.com")
	os.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer token,stream-name=default")
	os.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "45s")
	os.Setenv("OTEL_SERVICE_NAME", "hourglass-test")
	os.Setenv("OTEL_SERVICE_VERSION", "2.0.0")
	os.Setenv("DEPLOYMENT_ENVIRONMENT", "staging")
	defer os.Clearenv()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.Equal(t, "https://test.com", cfg.HourglassURL)
	assert.Equal(t, true, cfg.Debug)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "0 8 * * *", cfg.ScheduleMorning)
	assert.Equal(t, "0 18 * * *", cfg.ScheduleEvening)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, "test-xsrf-token", cfg.HourglassXSRFToken)
	assert.Equal(t, "test-hglogin-cookie", cfg.HourglassHGLogin)
	assert.Equal(t, "/tmp/webauthn-credentials.json", cfg.WebAuthnCredentialsPath)
	assert.False(t, cfg.AutoRefreshTokens)
	assert.Equal(t, "postgres://test:test@localhost:5432/hourglass", cfg.DatabaseURL)
	assert.Equal(t, "https://otel.example.com", cfg.OTLPEndpoint)
	assert.Equal(t, "Authorization=Bearer token,stream-name=default", cfg.OTLPHeaders)
	assert.Equal(t, 45*time.Second, cfg.OTLPMetricInterval)
	assert.Equal(t, "hourglass-test", cfg.TelemetryServiceName)
	assert.Equal(t, "2.0.0", cfg.TelemetryServiceVersion)
	assert.Equal(t, "staging", cfg.DeploymentEnvironment)
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
