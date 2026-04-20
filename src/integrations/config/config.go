// Package config provides environment-based application configuration.
package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	HourglassURL    string        `env:"HOURGLASS_URL" envDefault:"https://app.hourglass-app.com"`
	Debug           bool          `env:"DEBUG" envDefault:"false"`
	LogLevel        string        `env:"LOG_LEVEL" envDefault:"info"`
	ScheduleMorning string        `env:"SCHEDULE_MORNING" envDefault:"0 9 * * *"`
	ScheduleEvening string        `env:"SCHEDULE_EVENING" envDefault:"0 17 * * *"`
	Timeout         time.Duration `env:"TIMEOUT" envDefault:"60s"`
	// Hourglass API Authentication
	HourglassXSRFToken      string `env:"HOURGLASS_XSRF_TOKEN"`
	HourglassHGLogin        string `env:"HOURGLASS_HGLOGIN_COOKIE"`
	TokensPath              string `env:"TOKENS_PATH"`
	WebAuthnCredentialsPath string `env:"WEBAUTHN_CREDENTIALS_PATH"`
	ChromeProfileDir        string `env:"CHROME_PROFILE_DIR"`
	AutoRefreshTokens       bool   `env:"AUTO_REFRESH_TOKENS" envDefault:"true"`
	// Playwright Authentication
	HourglassEmail    string `env:"HOURGLASS_EMAIL"`
	HourglassPassword string `env:"HOURGLASS_PASSWORD"`
	// Database configuration
	DatabaseURL string `env:"DATABASE_URL"`
	// OpenTelemetry configuration
	OTLPEndpoint            string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTLPHeaders             string        `env:"OTEL_EXPORTER_OTLP_HEADERS"`
	OTLPMetricInterval      time.Duration `env:"OTEL_METRIC_EXPORT_INTERVAL" envDefault:"30s"`
	TelemetryServiceName    string        `env:"OTEL_SERVICE_NAME" envDefault:"hourglass-rejections-rpa"`
	TelemetryServiceVersion string        `env:"OTEL_SERVICE_VERSION" envDefault:"1.0.0"`
	DeploymentEnvironment   string        `env:"DEPLOYMENT_ENVIRONMENT" envDefault:"production"`
	// Telegram Bot configuration
	TelegramBotToken  string `env:"TELEGRAM_BOT_TOKEN"`
	TelegramWhitelist string `env:"TELEGRAM_WHITELIST"`
}

// Load parses environment variables and returns a Config instance.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
