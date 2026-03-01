package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"hourglass-rejeicoes-rpa/internal/config"
	"hourglass-rejeicoes-rpa/internal/logger"
	"hourglass-rejeicoes-rpa/internal/rpa"
	"hourglass-rejeicoes-rpa/internal/scheduler"
	"hourglass-rejeicoes-rpa/internal/sentry"
	"hourglass-rejeicoes-rpa/internal/storage"
)

func main() {
	var (
		setupMode = flag.Bool("setup", false, "Run in setup mode for manual login")
		onceMode  = flag.Bool("once", false, "Run once and exit (don't start scheduler)")
	)
	flag.Parse()

	// Initialize logger with charmbracelet/log
	logCfg := logger.ForTerminal()
	logCfg.Level = os.Getenv("LOG_LEVEL")
	if logCfg.Level == "" {
		logCfg.Level = "info"
	}
	logger := logger.New(logCfg)
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize Sentry
	sentryClient, err := sentry.New(sentry.Config{
		DSN:         cfg.SentryDSN,
		Environment: cfg.SentryEnvironment,
		Release:     "1.0.0",
	})
	if err != nil {
		logger.Error("failed to initialize sentry", "error", err)
	} else if sentryClient.IsEnabled() {
		logger.Info("sentry initialized successfully")
		defer sentryClient.Close()
	}

	logger.Info("starting hourglass-rejeicoes-rpa",
		"version", "1.0.0",
		"setup_mode", *setupMode,
		"once_mode", *onceMode,
	)

	// Initialize components
	browser := rpa.NewBrowser()
	store := storage.New(cfg)
	loginManager := rpa.NewLoginManager(browser, store)
	analyzer := rpa.NewAnalyzer(browser, loginManager)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup mode: run browser in non-headless mode for manual login
	if *setupMode {
		logger.Info("running in setup mode - please login manually")
		if err := runSetupMode(ctx, browser, loginManager); err != nil {
			logger.Error("setup mode failed", "error", err)
			os.Exit(1)
		}
		logger.Info("setup complete - cookies saved")
		return
	}

	// Once mode: run analysis once and exit
	if *onceMode {
		logger.Info("running in once mode")
		if err := runOnce(ctx, browser, loginManager, analyzer, store); err != nil {
			logger.Error("run failed", "error", err)
			os.Exit(1)
		}
		logger.Info("analysis complete")
		return
	}

	// Scheduler mode: run jobs at scheduled times
	logger.Info("starting scheduler mode")
	if err := runScheduler(ctx, browser, loginManager, analyzer, store); err != nil {
		logger.Error("scheduler failed", "error", err)
		os.Exit(1)
	}
}

func runSetupMode(ctx context.Context, browser *rpa.Browser, loginManager *rpa.LoginManager) error {
	if err := browser.Setup(); err != nil {
		return fmt.Errorf("failed to setup browser: %w", err)
	}
	defer browser.Close()

	if err := loginManager.PerformLogin(ctx); err != nil {
		return fmt.Errorf("failed to perform login: %w", err)
	}

	// Wait for user to complete login manually
	if err := loginManager.WaitForManualLogin(ctx); err != nil {
		return fmt.Errorf("failed to wait for manual login: %w", err)
	}

	if err := loginManager.SaveCookies(ctx); err != nil {
		return fmt.Errorf("failed to save cookies: %w", err)
	}

	return nil
}

func runOnce(ctx context.Context, browser *rpa.Browser, loginManager *rpa.LoginManager, analyzer *rpa.Analyzer, store *storage.FileStorage) error {
	if err := browser.Setup(); err != nil {
		return fmt.Errorf("failed to setup browser: %w", err)
	}
	defer browser.Close()

	// Load cookies if available
	if err := loginManager.LoadCookies(ctx); err != nil {
		slog.Warn("failed to load cookies", "error", err)
	}

	// Analyze all sections
	sections := []string{"Partes Mecânicas", "Campo", "Testemunho Público"}

	for _, section := range sections {
		slog.Info("analyzing section", "section", section)

		result, err := analyzer.AnalyzeSection(ctx, section)
		if err != nil {
			slog.Error("failed to analyze section", "section", section, "error", err)
			continue
		}

		if result.Error != nil {
			slog.Error("analysis returned error", "section", section, "error", result.Error)
			continue
		}

		slog.Info("section analysis complete",
			"section", section,
			"total", result.Total,
			"duration", result.Duration,
		)

		// Save results
		if len(result.Rejeicoes) > 0 {
			if err := store.Save(ctx, result.Rejeicoes); err != nil {
				slog.Error("failed to save results", "section", section, "error", err)
			}
		}
	}

	return nil
}

func runScheduler(ctx context.Context, browser *rpa.Browser, loginManager *rpa.LoginManager, analyzer *rpa.Analyzer, store *storage.FileStorage) error {
	logger := slog.Default()

	// Create scheduler
	s := scheduler.New(logger)

	// Create job function
	jobFunc := func(ctx context.Context) error {
		logger.Info("running scheduled analysis")
		return runOnce(ctx, browser, loginManager, analyzer, store)
	}

	// Schedule jobs for 9:00 AM and 5:00 PM
	if err := s.AddDailyJob("morning-analysis", 9, 0, jobFunc); err != nil {
		return fmt.Errorf("failed to schedule morning job: %w", err)
	}

	if err := s.AddDailyJob("evening-analysis", 17, 0, jobFunc); err != nil {
		return fmt.Errorf("failed to schedule evening job: %w", err)
	}

	// List scheduled jobs
	jobs := s.ListJobs()
	logger.Info("scheduled jobs", "count", len(jobs), "jobs", jobs)

	// Start scheduler
	s.Start()
	defer s.Stop()

	logger.Info("scheduler running - press Ctrl+C to stop")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		logger.Info("received shutdown signal")
	case <-ctx.Done():
		logger.Info("context cancelled")
	}

	return nil
}
