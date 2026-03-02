package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"hourglass-rejections-rpa/internal/api"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/logger"
	"hourglass-rejections-rpa/internal/notifier"
	"hourglass-rejections-rpa/internal/preferences"
	"hourglass-rejections-rpa/internal/scheduler"
	"hourglass-rejections-rpa/internal/sentry"
	"hourglass-rejections-rpa/internal/storage"
)

// init loads .env file if it exists
func init() {
	// Try to load .env from multiple possible locations
	// Silently ignore if not found (allows using system env vars)
	loadEnvFiles()
}

// loadEnvFiles attempts to load .env files from various locations
func loadEnvFiles() {
	// Possible locations for .env file
	locations := []string{
		".",       // Current directory
		"../.",    // Parent directory
		"../../.", // Grandparent directory
		filepath.Join(os.Getenv("HOME"), ".hourglass-rpa", "."), // Home directory
	}

	for _, location := range locations {
		if _, err := os.Stat(location); err == nil {
			if err := godotenv.Load(location); err == nil {
				// Successfully loaded
				return
			}
		}
	}

	// Try default . (will silently fail if not exists)
	_ = godotenv.Load()
}

func main() {
	var (
		onceMode = flag.Bool("once", false, "Run once and exit (don't start scheduler)")
		botMode  = flag.Bool("bot", false, "Start Telegram bot for interactive configuration")
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
		"bot_mode", *botMode,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bot mode: start Telegram bot for interactive configuration
	if *botMode {
		logger.Info("starting bot mode")
		if err := runBot(ctx, cfg); err != nil {
			logger.Error("bot failed", "error", err)
			os.Exit(1)
		}
		return
	}
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

	// Once mode: run analysis once and exit
	if *onceMode {
		logger.Info("running in once mode")
		if err := runOnce(ctx, cfg, sentryClient, analyzer, store); err != nil {
			logger.Error("run failed", "error", err)
			if sentryClient.IsEnabled() {
				sentryClient.Flush(2 * time.Second)
			}
			os.Exit(1)
		}
		logger.Info("analysis complete")
		return
	}

	// Scheduler mode: run jobs at scheduled times
	logger.Info("starting scheduler mode")
	if err := runScheduler(ctx, cfg, sentryClient, analyzer, store); err != nil {
		logger.Error("scheduler failed", "error", err)
		if sentryClient.IsEnabled() {
			sentryClient.Flush(2 * time.Second)
		}
		os.Exit(1)
	}
}

func runOnce(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	// Analyze all sections
	sections := []string{"Partes Mecânicas", "Campo", "Testemunho Público", "Reunião Meio de Semana"}
	var allRejections []domain.Rejeicao

	for _, section := range sections {
		slog.Info("analyzing section", "section", section)

		result, err := analyzer.AnalyzeSection(section)
		if err != nil {
			slog.Error("failed to analyze section", "section", section, "error", err)
			if sentryClient.IsEnabled() {
				sentryClient.CaptureError(err, map[string]interface{}{
					"section":   section,
					"operation": "analyze_section",
				})
			}
			continue
		}

		if result.Error != nil {
			slog.Error("analysis returned error", "section", section, "error", result.Error)
			if sentryClient.IsEnabled() {
				sentryClient.CaptureError(result.Error, map[string]interface{}{
					"section":   section,
					"operation": "analyze_section_result",
				})
			}
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
				if sentryClient.IsEnabled() {
					sentryClient.CaptureError(err, map[string]interface{}{
						"section":   section,
						"operation": "save_results",
					})
				}
			}
		}
	}

	// Send Telegram notifications (filtered per user if preferences configured)
	if len(allRejections) > 0 {
		prefStore := preferences.NewFilePreferenceStore(cfg.UserPrefsFile)
		prefManager := preferences.NewPreferenceManager(prefStore)

		// Try filtered notifications first; fallback to broadcast if no users configured
		users, listErr := prefManager.List()
		if listErr != nil {
			slog.Warn("failed to list user preferences, falling back to broadcast", "error", listErr)
			if err := sendTelegramNotification(sentryClient, allRejections); err != nil {
				slog.Error("failed to send telegram notification", "error", err)
			}
		} else if len(users) == 0 {
			slog.Info("no user preferences configured, using broadcast notification")
			if err := sendTelegramNotification(sentryClient, allRejections); err != nil {
				slog.Error("failed to send telegram notification", "error", err)
			}
		} else {
			if err := sendFilteredNotifications(sentryClient, allRejections, prefManager); err != nil {
				slog.Error("failed to send filtered notifications", "error", err)
			}
		}
	}

	return nil
}

func sendTelegramNotification(sentryClient *sentry.Client, rejections []domain.Rejeicao) error {
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

// Parse chat IDs (supports comma-separated list)
var chatIDs []int64
for _, idStr := range strings.Split(chatIDStr, ",") {
id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
if err != nil {
slog.Warn("invalid chat ID in TELEGRAM_CHAT_ID, skipping", "id", idStr, "error", err)
continue
}
chatIDs = append(chatIDs, id)
}

if len(chatIDs) == 0 {
return fmt.Errorf("no valid telegram chat IDs found")
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

tgBot, err := notifier.NewTelegramNotifier(token, chatIDs[0], whitelist)
if err != nil {
if sentryClient != nil && sentryClient.IsEnabled() {
sentryClient.CaptureError(err, map[string]interface{}{
"operation": "create_telegram_notifier",
})
}
return fmt.Errorf("failed to create telegram notifier: %w", err)
}

// Send notification to each chat ID
var sentCount int
for _, chatID := range chatIDs {
if !tgBot.IsAuthorized(chatID) {
slog.Warn("unauthorized chat ID, skipping notification", "chat_id", chatID)
continue
}
if err := tgBot.SendRejectionsNotification(chatID, rejections); err != nil {
slog.Error("failed to send telegram notification to chat", "chat_id", chatID, "error", err)
if sentryClient != nil && sentryClient.IsEnabled() {
sentryClient.CaptureError(err, map[string]interface{}{
"operation": "send_telegram_notification",
"chat_id": chatID,
})
}
continue
}
sentCount++
slog.Info("telegram notification sent successfully", "chat_id", chatID, "count", len(rejections))
}

if sentCount == 0 {
return fmt.Errorf("failed to send telegram notification to any chat")
}

return nil
}

// sendFilteredNotifications sends per-user filtered notifications based on section preferences.
func sendFilteredNotifications(
	sentryClient *sentry.Client,
	rejections []domain.Rejeicao,
	prefManager *preferences.PreferenceManager,
) error {
	if len(rejections) == 0 {
		return nil
	}

	// Group rejections by section for efficient filtering
	bySection := make(map[string][]domain.Rejeicao)
	for _, r := range rejections {
		bySection[r.Secao] = append(bySection[r.Secao], r)
	}

	// Get all users with preferences
	users, err := prefManager.List()
	if err != nil {
		return fmt.Errorf("failed to list user preferences: %w", err)
	}

	// Create Telegram notifier
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		slog.Warn("Telegram bot token not configured, skipping filtered notifications")
		return nil
	}

	// Use a dummy chat ID for notifier creation (actual sending uses per-user chat IDs)
	tgBot, err := notifier.NewTelegramNotifier(token, users[0].ChatID, nil)
	if err != nil {
		if sentryClient != nil && sentryClient.IsEnabled() {
			sentryClient.CaptureError(err, map[string]interface{}{
				"operation": "create_telegram_notifier_filtered",
			})
		}
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	var sentCount int
	for _, user := range users {
		if !user.Enabled {
			slog.Debug("user notifications disabled, skipping", "chat_id", user.ChatID, "username", user.Username)
			continue
		}

		// Collect rejections from user's selected sections
		var userRejections []domain.Rejeicao
		for _, section := range user.Sections {
			userRejections = append(userRejections, bySection[section]...)
		}

		if len(userRejections) == 0 {
			slog.Debug("no rejections for user's sections, skipping", "chat_id", user.ChatID, "username", user.Username)
			continue
		}

		// Send to this specific user
		if err := tgBot.SendRejectionsNotification(user.ChatID, userRejections); err != nil {
			slog.Error("failed to send filtered notification",
				"chat_id", user.ChatID,
				"username", user.Username,
				"rejection_count", len(userRejections),
				"error", err,
			)
			if sentryClient != nil && sentryClient.IsEnabled() {
				sentryClient.CaptureError(err, map[string]interface{}{
					"operation": "send_filtered_notification",
					"chat_id":   user.ChatID,
					"username":  user.Username,
				})
			}
			continue
		}

		sentCount++
		slog.Info("filtered notification sent successfully",
			"chat_id", user.ChatID,
			"username", user.Username,
			"rejection_count", len(userRejections),
		)
	}

	slog.Info("filtered notifications complete", "sent", sentCount, "total_users", len(users))
	return nil
}

func runScheduler(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
	logger := slog.Default()

	// Create scheduler
	s := scheduler.New(logger)

	// Create job function
	jobFunc := func(ctx context.Context) error {
		logger.Info("running scheduled analysis")
		return runOnce(ctx, cfg, sentryClient, analyzer, store)
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

func runBot(ctx context.Context, cfg *config.Config) error {
	logger := slog.Default()

	// Validate Telegram configuration
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN not configured")
	}

	// Initialize preference manager
	prefStore := preferences.NewFilePreferenceStore(cfg.UserPrefsFile)
	prefManager := preferences.NewPreferenceManager(prefStore)

	// Parse chat IDs from env (for whitelist)
	var whitelist []int64
	if chatIDs := os.Getenv("TELEGRAM_CHAT_ID"); chatIDs != "" {
		for _, idStr := range strings.Split(chatIDs, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				logger.Warn("invalid chat ID in env, skipping", "id", idStr, "error", err)
				continue
			}
			whitelist = append(whitelist, id)
		}
	}
	
	// Create Telegram notifier (for bot mode, we just need the bot instance)
	// Use first chat ID as default, or 0 if none
	var chatID int64
	if len(whitelist) > 0 {
		chatID = whitelist[0]
	}
	
	tgBot, err := notifier.NewTelegramNotifier(token, chatID, whitelist)
	if err != nil {
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}
	
	logger.Info("starting telegram bot", "whitelist_count", len(whitelist))
	
	// Start bot in listener mode
	if err := tgBot.StartBot(ctx, prefManager); err != nil {
		return fmt.Errorf("failed to start bot: %w", err)
	}
	
	logger.Info("bot started successfully - send /start to your bot")
	
	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	select {
	case <-sigChan:
		logger.Info("received shutdown signal")
	case <-ctx.Done():
		logger.Info("context cancelled")
	}
	
	// Stop bot gracefully
	if err := tgBot.StopBot(); err != nil {
		logger.Error("error stopping bot", "error", err)
	}
	
	logger.Info("bot stopped")
	return nil
}
