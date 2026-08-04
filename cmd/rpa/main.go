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

	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/logger"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"
	"hourglass-rejections-rpa/src/services/bot"
	"hourglass-rejections-rpa/src/services/hourglass"
	"hourglass-rejections-rpa/src/services/scheduler"
)

type runOptions struct {
	args   []string
	getenv func(string) string
	exit   func(int)
}

var osExit = os.Exit

var (
	version  = "dev"
	revision = "unknown"
)

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

var telemetryClientGlobal *telemetry.Client
var enableWebAuthnClient = func(apiClient *hourglass.Client, credentialsPath string) error {
	return apiClient.EnableWebAuthn(credentialsPath, captureError)
}

func captureError(err error, extras map[string]any) {
	if telemetryClientGlobal != nil && telemetryClientGlobal.IsEnabled() {
		telemetryClientGlobal.CaptureError(err, extras)
		telemetryClientGlobal.Flush(2 * time.Second)
	}
}

func run(ctx context.Context, opts runOptions) error {
	fs := flag.NewFlagSet("rpa", flag.ContinueOnError)
	onceMode := fs.Bool("once", false, "Run once and exit")

	if err := fs.Parse(opts.args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	setupLogging(opts.getenv("LOG_LEVEL"))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	telemetryClient := setupTelemetry(cfg)
	slog.SetDefault(telemetryClient.Logger())
	telemetryClientGlobal = telemetryClient
	if telemetryClient.IsEnabled() {
		defer telemetryClient.Close()
	}

	slog.Info("starting hourglass-rejections-rpa", "version", version, "revision", revision, "once_mode", *onceMode)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	apiClient, analyzer, store, err := setupDependencies(cfg)
	if err != nil {
		return fmt.Errorf("failed to setup dependencies: %w", err)
	}

	if err := apiClient.StartTokenManager(ctx); err != nil {
		return fmt.Errorf("failed to start token manager: %w", err)
	}
	defer apiClient.StopTokenManager()

	if *onceMode {
		slog.Info("running in once mode")
		return runOnceMode(ctx, cfg, telemetryClient, analyzer, store)
	}

	return runFullMode(ctx, cfg, telemetryClient, analyzer, store)
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

func setupTelemetry(cfg *config.Config) *telemetry.Client {
	client, err := telemetry.New(telemetry.Config{
		Endpoint:       cfg.OTLPEndpoint,
		Headers:        cfg.OTLPHeaders,
		Environment:    cfg.DeploymentEnvironment,
		ServiceName:    cfg.TelemetryServiceName,
		Release:        cfg.TelemetryServiceVersion,
		MetricInterval: cfg.OTLPMetricInterval,
		Level:          cfg.LogLevel,
	})
	if err != nil {
		setupLogging(cfg.LogLevel)
		fallback, _ := telemetry.New(telemetry.Config{Level: cfg.LogLevel})
		return fallback
	}
	return client
}

func setupDependencies(cfg *config.Config) (*hourglass.Client, *hourglass.APIAnalyzer, *preferences.Store, error) {
	bundle, err := buildDependencies(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	return bundle.apiClient, bundle.analyzer, bundle.store, nil
}

func resolveTokensPath(cfg *config.Config) string {
	if cfg.TokensPath != "" {
		return cfg.TokensPath
	}

	if tokensPath := os.Getenv("TOKENS_PATH"); tokensPath != "" {
		return tokensPath
	}

	if tokensPath := os.Getenv("WEBAUTHN_TOKENS_PATH"); tokensPath != "" {
		return tokensPath
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".hourglass-rpa", "auth-tokens.json")
	}

	return ""
}

func resolveWebAuthnCredentialsPath(cfg *config.Config) string {
	if cfg.WebAuthnCredentialsPath != "" {
		return cfg.WebAuthnCredentialsPath
	}

	if credentialsPath := os.Getenv("WEBAUTHN_CREDENTIALS_PATH"); credentialsPath != "" {
		return credentialsPath
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".hourglass-rpa", "webauthn-credentials.json")
	}

	return ""
}

func resolveChromeProfileDir(cfg *config.Config) string {
	if cfg.ChromeProfileDir != "" {
		return cfg.ChromeProfileDir
	}

	if profileDir := os.Getenv("CHROME_PROFILE_DIR"); profileDir != "" {
		return profileDir
	}

	return ""
}

func enableWebAuthnTokenManager(apiClient *hourglass.Client, cfg *config.Config) bool {
	if !cfg.AutoRefreshTokens {
		slog.Info("automatic token renewal disabled")
		return false
	}

	credentialsPath := resolveWebAuthnCredentialsPath(cfg)
	profileDir := resolveChromeProfileDir(cfg)
	if credentialsPath == "" && profileDir == "" {
		return false
	}

	credentialsAvailable := false
	if credentialsPath != "" {
		_, err := os.Stat(credentialsPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if profileDir == "" {
					slog.Info("authentication credentials not found, using static token flow", "path", credentialsPath)
					return false
				}
				slog.Info("webauthn credentials not found, continuing with persistent browser profile auth", "path", credentialsPath, "profile_dir", profileDir)
			} else {
				if profileDir == "" {
					slog.Warn("failed to inspect webauthn credentials path", "path", credentialsPath, "error", err)
					return false
				}
				slog.Warn("failed to inspect webauthn credentials path, continuing with persistent browser profile auth", "path", credentialsPath, "profile_dir", profileDir, "error", err)
			}
		} else {
			credentialsAvailable = true
		}
	}

	if err := enableWebAuthnClient(apiClient, credentialsPath); err != nil {
		slog.Warn("failed to enable automatic authentication token manager", "path", credentialsPath, "profile_dir", profileDir, "error", err)
		return false
	}

	slog.Info("enabled automatic authentication token manager", "credentials_path", credentialsPath, "credentials_available", credentialsAvailable, "browser_profile_dir", profileDir)
	return true
}

var runOnceFn = func(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
	return errors.New("runOnce not implemented")
}

type runner interface {
	Run(ctx context.Context) error
}

var newSchedulerFn = func(cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) runner {
	return scheduler.New(cfg, telemetryClient, analyzer, store)
}

func runOnceMode(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
	if err := runOnceFn(ctx, cfg, telemetryClient, analyzer, store); err != nil {
		telemetryClient.CaptureError(err, map[string]any{
			"phase": "run_once_mode",
		})
		return fmt.Errorf("run failed: %w", err)
	}
	return nil
}

func runFullMode(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
	slog.Info("starting full mode (scheduler + bot)")

	go func() {
		botRunner := bot.New(cfg, telemetryClient, analyzer).WithPreferenceStore(store)
		if err := botRunner.Run(ctx); err != nil {
			slog.Error("bot error", "error", err)
			telemetryClient.CaptureError(err, map[string]any{
				"phase": "bot_run",
			})
		}
	}()

	sched := newSchedulerFn(cfg, telemetryClient, analyzer, store)
	if err := sched.Run(ctx); err != nil {
		telemetryClient.CaptureError(err, map[string]any{
			"phase": "scheduler_run",
		})
		return fmt.Errorf("scheduler failed: %w", err)
	}

	return nil
}
