package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"hourglass-rejections-rpa/internal/api"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/logger"
	"hourglass-rejections-rpa/internal/notifier"
	"hourglass-rejections-rpa/internal/scheduler"
	"hourglass-rejections-rpa/internal/sentry"
	"hourglass-rejections-rpa/internal/storage"
)

func main() {
	var (
		onceMode = flag.Bool("once", false, "Run once and exit (don't start scheduler)")
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

	logger.Info("starting hourglass-rejections-rpa",
		"version", "1.0.0",
		"once_mode", *onceMode,
	)

	// Initialize API client
	apiClient := api.NewClient()

	// Set XSRF token from environment if available
	if xsrfToken := os.Getenv("HOURGLASS_XSRF_TOKEN"); xsrfToken != "" {
		apiClient.SetXSRFToken(xsrfToken)
		logger.Info("XSRF token configured")
	}

	// Set HGLogin cookie from environment if available
	if hglogin := os.Getenv("HOURGLASS_HGLOGIN_COOKIE"); hglogin != "" {
		apiClient.SetHGLogin(hglogin)
		logger.Info("HGLogin cookie configured")
	}

	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Once mode: run analysis once and exit
	if *onceMode {
		logger.Info("running in once mode")
		if err := runOnce(ctx, analyzer, store); err != nil {
			logger.Error("run failed", "error", err)
			os.Exit(1)
		}
		logger.Info("analysis complete")
		return
	}

	// Scheduler mode: run jobs at scheduled times
	logger.Info("starting scheduler mode")
	if err := runScheduler(ctx, analyzer, store); err != nil {
		logger.Error("scheduler failed", "error", err)
		os.Exit(1)
	}
}

func runOnce(ctx context.Context, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	// Analyze all sections
	sections := []string{"Partes Mecânicas", "Campo", "Testemunho Público"}
	var allRejections []domain.Rejeicao

	for _, section := range sections {
		slog.Info("analyzing section", "section", section)

		result, err := analyzer.AnalyzeSection(section)
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
			allRejections = append(allRejections, result.Rejeicoes...)
			if err := store.Save(ctx, result.Rejeicoes); err != nil {
				slog.Error("failed to save results", "section", section, "error", err)
			}
		}
	}

	// Send Telegram notification if there are rejections
	if len(allRejections) > 0 {
		if err := sendTelegramNotification(allRejections); err != nil {
			slog.Error("failed to send telegram notification", "error", err)
		}
	}

	return nil
}

func sendTelegramNotification(rejections []domain.Rejeicao) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	whitelistStr := os.Getenv("TELEGRAM_WHITELIST")

	if token == "" || chatIDStr == "" {
		slog.Warn("Telegram configuration missing, skipping notification",
			"has_token", token != "",
			"has_chat_id", chatIDStr != "",
		)
		return nil
	}

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat ID: %w", err)
	}

	var whitelist []int64
	if whitelistStr != "" {
		for _, idStr := range strings.Split(whitelistStr, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				slog.Warn("invalid chat ID in whitelist, skipping", "id", idStr, "error", err)
				continue
			}
			whitelist = append(whitelist, id)
		}
	}

	tgBot, err := notifier.NewTelegramNotifier(token, chatID, whitelist)
	if err != nil {
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	if !tgBot.IsAuthorized(chatID) {
		slog.Warn("unauthorized chat ID, skipping notification", "chat_id", chatID)
		return nil
	}

	if err := tgBot.SendRejectionsNotification(rejections); err != nil {
		return fmt.Errorf("failed to send telegram notification: %w", err)
	}

	slog.Info("telegram notification sent successfully", "count", len(rejections))
	return nil
}

func runScheduler(ctx context.Context, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	logger := slog.Default()

	// Create scheduler
	s := scheduler.New(logger)

	// Create job function
	jobFunc := func(ctx context.Context) error {
		logger.Info("running scheduled analysis")
		return runOnce(ctx, analyzer, store)
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
