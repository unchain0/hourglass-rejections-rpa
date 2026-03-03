package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"

	"hourglass-rejections-rpa/internal/api"
	"hourglass-rejections-rpa/internal/bot"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/logger"
	"hourglass-rejections-rpa/internal/scheduler"
	"hourglass-rejections-rpa/internal/sentry"
	"hourglass-rejections-rpa/internal/storage"
)

type runOptions struct {
	args   []string
	getenv func(string) string
	exit   func(int)
}

var osExit = os.Exit

func init() {
	loadEnvFiles()
}

func loadEnvFiles() {
	locations := []string{
		".env",
		"../.env",
		"../../.env",
		filepath.Join(os.Getenv("HOME"), ".hourglass-rpa", ".env"),
	}

	for _, location := range locations {
		if _, err := os.Stat(location); err == nil {
			if err := godotenv.Load(location); err == nil {
				return
			}
		}
	}
	_ = godotenv.Load()
}

func main() {
	opts := runOptions{
		args:   os.Args[1:],
		getenv: os.Getenv,
		exit:   osExit,
	}

	if err := run(context.Background(), opts); err != nil {
		if err.Error() != "" {
			slog.Error("application error", "error", err)
		}
		opts.exit(1)
	}
}

var sentryClientGlobal *sentry.Client

func captureError(err error, extras map[string]interface{}) {
	if sentryClientGlobal != nil && sentryClientGlobal.IsEnabled() {
		sentryClientGlobal.CaptureError(err, extras)
		sentryClientGlobal.Flush(2 * time.Second)
	}
}

func run(ctx context.Context, opts runOptions) error {
	fs := flag.NewFlagSet("rpa", flag.ContinueOnError)
	onceMode := fs.Bool("once", false, "Run once and exit")

	if err := fs.Parse(opts.args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	setupLogging(opts.getenv("LOG_LEVEL"))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	sentryClient := setupSentry(cfg)
	sentryClientGlobal = sentryClient
	if sentryClient.IsEnabled() {
		defer sentryClient.Close()
	}

	slog.Info("starting hourglass-rejections-rpa", "version", "1.0.0", "once_mode", *onceMode)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	analyzer, store := setupDependencies(cfg)

	if *onceMode {
		slog.Info("running in once mode")
		return runOnceMode(ctx, cfg, sentryClient, analyzer, store)
	}

	return runFullMode(ctx, cfg, sentryClient, analyzer, store)
}

func setupLogging(level string) {
	logCfg := logger.ForTerminal()
	logCfg.Level = level
	if logCfg.Level == "" {
		logCfg.Level = "info"
	}
	l := logger.New(logCfg)
	slog.SetDefault(l)
}

func setupSentry(cfg *config.Config) *sentry.Client {
	client, _ := sentry.New(sentry.Config{
		DSN:         cfg.SentryDSN,
		Environment: cfg.SentryEnvironment,
		Release:     "1.0.0",
	})
	return client
}

func setupDependencies(cfg *config.Config) (*api.APIAnalyzer, *storage.FileStorage) {
	apiClient := api.NewClient()
	if cfg.HourglassXSRFToken != "" {
		apiClient.SetXSRFToken(cfg.HourglassXSRFToken)
	}
	if cfg.HourglassHGLogin != "" {
		apiClient.SetHGLogin(cfg.HourglassHGLogin)
	}

	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)
	return analyzer, store
}

var runOnceFn = func(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	return errors.New("runOnce not implemented")
}

type runner interface {
	Run(ctx context.Context) error
}

var newSchedulerFn = func(cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) runner {
	return scheduler.New(cfg, sentryClient, analyzer, store)
}

func runOnceMode(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	if err := runOnceFn(ctx, cfg, sentryClient, analyzer, store); err != nil {
		sentryClient.CaptureError(err, map[string]interface{}{
			"phase": "run_once_mode",
		})
		return fmt.Errorf("run failed: %w", err)
	}
	return nil
}

func runFullMode(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	slog.Info("starting full mode (scheduler + bot)")

	go func() {
		botRunner := bot.New(cfg, sentryClient, analyzer, store)
		if err := botRunner.Run(ctx); err != nil {
			slog.Error("bot error", "error", err)
			sentryClient.CaptureError(err, map[string]interface{}{
				"phase": "bot_run",
			})
		}
	}()

	sched := newSchedulerFn(cfg, sentryClient, analyzer, store)
	if err := sched.Run(ctx); err != nil {
		sentryClient.CaptureError(err, map[string]interface{}{
			"phase": "scheduler_run",
		})
		return fmt.Errorf("scheduler failed: %w", err)
	}

	return nil
}
