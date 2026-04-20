package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	domain "hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/i18n"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"
	"hourglass-rejections-rpa/src/services/hourglass"
	notifier "hourglass-rejections-rpa/src/services/notification"
)

type Analyzer interface {
	AnalyzeSection(section string) (*domain.JobResult, error)
}

type Notifier interface {
	StartBot(ctx context.Context, prefManager *preferences.PreferenceManager) error
	StopBot() error
	SetCheckNowCallback(callback notifier.CheckNowCallback)
	SendNoRejectionsMessage(chatID int64, message string) error
	SendRejectionsNotification(chatID int64, rejections []domain.Rejection) error
}

type BotRunner struct {
	cfg             *config.Config
	telemetryClient *telemetry.Client
	analyzer        Analyzer
	mu              sync.RWMutex

	notifier  Notifier
	prefStore preferences.PreferenceStore
}

// New creates a bot runner wired to the shared analyzer and telemetry pipeline.
func New(cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer) *BotRunner {
	return &BotRunner{
		cfg:             cfg,
		telemetryClient: telemetryClient,
		analyzer:        analyzer,
	}
}

func (b *BotRunner) WithNotifier(n Notifier) *BotRunner {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notifier = n
	return b
}

func (b *BotRunner) WithPreferenceStore(s preferences.PreferenceStore) *BotRunner {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prefStore = s
	return b
}

func (b *BotRunner) WithAnalyzer(a Analyzer) *BotRunner {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.analyzer = a
	return b
}

var newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
	return notifier.NewTelegramNotifier(token, chatID, whitelist)
}

var newPreferenceStoreFromDatabaseURL = func(databaseURL string) (preferences.PreferenceStore, error) {
	return preferences.NewStoreFromConfig(&preferences.DatabaseConfig{Type: "postgres", DSN: databaseURL})
}

var i18nInit = i18n.Init

func (b *BotRunner) Run(ctx context.Context) error {
	logger := slog.Default()

	if err := i18nInit(); err != nil {
		b.telemetryClient.CaptureError(err, map[string]interface{}{
			"phase": "init_i18n",
		})
		return fmt.Errorf("failed to initialize i18n: %w", err)
	}

	prefStore, closePreferenceStore, err := b.ensurePreferenceStore()
	if err != nil {
		b.telemetryClient.CaptureError(err, map[string]interface{}{
			"phase":            "init_preference_store",
			"database_url_set": b.cfg != nil && b.cfg.DatabaseURL != "",
		})
		return fmt.Errorf("failed to initialize preference store: %w", err)
	}
	defer closePreferenceStore()

	prefManager := preferences.NewPreferenceManager(prefStore)

	tgBot, err := b.ensureNotifier()
	if err != nil {
		whitelist := b.getWhitelist()
		var chatID int64
		if len(whitelist) > 0 {
			chatID = whitelist[0]
		}
		b.telemetryClient.CaptureError(err, map[string]interface{}{
			"phase":     "create_notifier",
			"chat_id":   chatID,
			"has_token": b.getBotToken() != "",
		})
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	tgBot.SetCheckNowCallback(func(ctx context.Context, chatID int64) error {
		logger.Info("manual check triggered via bot", "chat_id", chatID)
		return b.runOnceForUser(ctx, prefManager, chatID)
	})

	logger.Info("starting telegram bot")

	if err := tgBot.StartBot(ctx, prefManager); err != nil {
		b.telemetryClient.CaptureError(err, map[string]interface{}{
			"phase": "start_bot",
		})
		return fmt.Errorf("failed to start bot: %w", err)
	}

	logger.Info("bot started successfully - send /start to your bot")

	<-ctx.Done()

	if err := tgBot.StopBot(); err != nil {
		logger.Error("error stopping bot", "error", err)
		b.telemetryClient.CaptureError(err, map[string]interface{}{
			"phase": "stop_bot",
		})
	}

	logger.Info("bot stopped")
	return nil
}

func (b *BotRunner) runOnceForUser(ctx context.Context, prefManager *preferences.PreferenceManager, targetChatID int64) error {
	service := newManualCheckService(b.analyzer, b.telemetryClient, prefManager, b.sendNoRejectionsMessage, b.sendRejectionsNotification)
	return service.run(ctx, targetChatID)
}

func (b *BotRunner) sendNoRejectionsMessage(chatID int64, message string) error {
	logger := slog.Default()
	logger.Info("sending no rejections message", "chat_id", chatID, "message", message)

	if b.notifier != nil {
		logger.Info("using existing notifier")
		return b.notifier.SendNoRejectionsMessage(chatID, message)
	}

	tgBot, err := b.newTransientNotifier(chatID)
	if err != nil {
		logger.Error("TELEGRAM_BOT_TOKEN not configured")
		if b.telemetryClient != nil && err.Error() != "TELEGRAM_BOT_TOKEN not configured" {
			b.telemetryClient.CaptureError(err, map[string]interface{}{
				"phase":   "create_temp_notifier",
				"chat_id": chatID,
			})
		}
		return err
	}

	return tgBot.SendNoRejectionsMessage(chatID, message)
}

func (b *BotRunner) sendRejectionsNotification(chatID int64, rejections []domain.Rejection) error {
	if b.notifier != nil {
		return b.notifier.SendRejectionsNotification(chatID, rejections)
	}

	tgBot, err := b.newTransientNotifier(chatID)
	if err != nil {
		return err
	}

	return tgBot.SendRejectionsNotification(chatID, rejections)
}

func (b *BotRunner) ensurePreferenceStore() (preferences.PreferenceStore, func(), error) {
	if b.prefStore != nil {
		return b.prefStore, func() {}, nil
	}
	if b.cfg == nil || b.cfg.DatabaseURL == "" {
		return nil, func() {}, fmt.Errorf("DATABASE_URL not configured")
	}

	store, err := newPreferenceStoreFromDatabaseURL(b.cfg.DatabaseURL)
	if err != nil {
		return nil, func() {}, err
	}
	prefStore := store

	closeStore := func() {}
	if closer, ok := any(prefStore).(interface{ Close() error }); ok {
		closeStore = func() { _ = closer.Close() }
	}

	return prefStore, closeStore, nil
}

func (b *BotRunner) ensureNotifier() (Notifier, error) {
	if b.notifier != nil {
		return b.notifier, nil
	}

	token := b.getBotToken()
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN not configured")
	}

	whitelist := b.getWhitelist()
	var chatID int64
	if len(whitelist) > 0 {
		chatID = whitelist[0]
	}

	tgBot, err := newTelegramNotifier(token, chatID, whitelist)
	if err != nil {
		return nil, err
	}

	b.notifier = tgBot
	return tgBot, nil
}

func (b *BotRunner) newTransientNotifier(chatID int64) (Notifier, error) {
	token := b.getBotToken()
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN not configured")
	}

	whitelist := append(b.getWhitelist(), chatID)
	slog.Default().Info("creating temporary notifier", "chat_id", chatID, "whitelist", whitelist)

	tgBot, err := newTelegramNotifier(token, chatID, whitelist)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	return tgBot, nil
}

func (b *BotRunner) getBotToken() string {
	if b.cfg != nil && b.cfg.TelegramBotToken != "" {
		return b.cfg.TelegramBotToken
	}

	return os.Getenv("TELEGRAM_BOT_TOKEN")
}
func (b *BotRunner) getWhitelist() []int64 {
	var whitelist []int64
	var whitelistEnv string
	if b.cfg != nil {
		whitelistEnv = b.cfg.TelegramWhitelist
	}
	if whitelistEnv == "" {
		whitelistEnv = os.Getenv("TELEGRAM_WHITELIST")
	}
	if whitelistEnv != "" {
		for _, idStr := range strings.Split(whitelistEnv, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				continue
			}
			whitelist = append(whitelist, id)
		}
	}
	return whitelist
}
