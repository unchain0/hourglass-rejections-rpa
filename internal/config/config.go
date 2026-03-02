package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HourglassURL    string        `env:"HOURGLASS_URL" envDefault:"https://app.hourglass-app.com"`
	CookieFile      string        `env:"COOKIE_FILE" envDefault:"cookies.json"`
	OutputDir       string        `env:"OUTPUT_DIR" envDefault:"./outputs"`
	Debug           bool          `env:"DEBUG" envDefault:"false"`
	ScheduleMorning string        `env:"SCHEDULE_MORNING" envDefault:"0 9 * * *"`
	ScheduleEvening string        `env:"SCHEDULE_EVENING" envDefault:"0 17 * * *"`
	Timeout         time.Duration `env:"TIMEOUT" envDefault:"60s"`
	// Hourglass API Authentication
	HourglassXSRFToken string `env:"HOURGLASS_XSRF_TOKEN"`
	HourglassHGLogin   string `env:"HOURGLASS_HGLOGIN_COOKIE"`
	// Playwright Authentication
	HourglassEmail    string `env:"HOURGLASS_EMAIL"`
	HourglassPassword string `env:"HOURGLASS_PASSWORD"`
	// User Preferences (SQLite is default)
	UseJSON       bool   `env:"USE_JSON" envDefault:"false"`
	UserPrefsFile string `env:"USER_PREFS_FILE" envDefault:"data/preferences.json"`
	SQLiteDBPath  string `env:"SQLITE_DB_PATH" envDefault:"data/hourglass.db"`
	// Sentry configuration
	SentryDSN         string `env:"SENTRY_DSN"`
	SentryEnvironment string `env:"SENTRY_ENVIRONMENT" envDefault:"production"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
