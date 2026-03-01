package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HourglassURL    string        `env:"HOURGLASS_URL" envDefault:"https://hourglass.petrobras.com"`
	CookieFile      string        `env:"COOKIE_FILE" envDefault:"cookies.json"`
	OutputDir       string        `env:"OUTPUT_DIR" envDefault:"./outputs"`
	Debug           bool          `env:"DEBUG" envDefault:"false"`
	ScheduleMorning string        `env:"SCHEDULE_MORNING" envDefault:"0 9 * * *"`
	ScheduleEvening string        `env:"SCHEDULE_EVENING" envDefault:"0 17 * * *"`
	Timeout         time.Duration `env:"TIMEOUT" envDefault:"60s"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
