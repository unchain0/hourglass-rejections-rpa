package bot

import (
	"context"
	"errors"
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

const telegramBotTokenNotConfigured = "TELEGRAM_BOT_TOKEN not configured"

// New creates a bot runner wired to the shared analyzer and telemetry pipeline.
func New(cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer) *BotRunner {
	return &BotRunner{
		cfg:             cfg,
		telemetryClient: telemetryClient,
		analyzer:        analyzer,
	}
}

// noOpPreferenceStoreClose intentionally keeps the same signature as the production close function
// while avoiding nil checks when no preference store was configured.
func noOpPreferenceStoreClose() {}

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

func openPreferenceStoreFromDatabaseURL(databaseURL string) (preferences.PreferenceStore, error) {
	return preferences.NewStoreFromConfig(&preferences.DatabaseConfig{Type: "postgres", DSN: databaseURL})
}

var newPreferenceStoreFromDatabaseURL = openPreferenceStoreFromDatabaseURL

var i18nInit = i18n.Init

func (b *BotRunner) Run(ctx context.Context) error {
	logger := slog.Default()

	if err := i18nInit(); err != nil {
		b.telemetryClient.CaptureError(err, map[string]any{
			"phase": "init_i18n",
		})
		return fmt.Errorf("failed to initialize i18n: %w", err)
	}

	prefStore, closePreferenceStore, err := b.ensurePreferenceStore()
	if err != nil {
		b.telemetryClient.CaptureError(err, map[string]any{
			"phase":            "init_preference_store",
			"database_url_set": b.cfg != nil && b.cfg.DatabaseURL != "",
		})
		return fmt.Errorf("failed to initialize preference store: %w", err)
	}
	defer closePreferenceStore()

	prefManager := preferences.NewPreferenceManager(prefStore)

	tgBot, err := b.ensureNotifier()
	if err != nil {
		b.telemetryClient.CaptureError(err, map[string]any{
			"phase":     "create_notifier",
			"has_token": b.getBotToken() != "",
		})
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	tgBot.SetCheckNowCallback(func(ctx context.Context, chatID int64) error {
		logger.Info("manual check triggered via bot")
		return b.runOnceForUser(ctx, prefManager, chatID)
	})

	logger.Info("starting telegram bot")

	if err := tgBot.StartBot(ctx, prefManager); err != nil {
		b.telemetryClient.CaptureError(err, map[string]any{
			"phase": "start_bot",
		})
		return fmt.Errorf("failed to start bot: %w", err)
	}

	logger.Info("bot started successfully - send /start to your bot")

	<-ctx.Done()

	if err := tgBot.StopBot(); err != nil {
		logger.Error("error stopping bot", "error", err)
		b.telemetryClient.CaptureError(err, map[string]any{
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

func (b *BotRunner) SendRejections(rejections []domain.Rejection) error {
	if len(rejections) == 0 {
		return nil
	}

	if b.prefStore == nil && (b.cfg == nil || b.cfg.DatabaseURL == "") {
		return fmt.Errorf("preference store not configured")
	}

	prefStore, closePrefStore, err := b.ensurePreferenceStore()
	if err != nil {
		return err
	}
	if prefStore == nil {
		return fmt.Errorf("preference store not configured")
	}
	if closePrefStore != nil {
		defer closePrefStore()
	}

	return b.sendRejectionsToUsers(prefStore, rejections, b.getWhitelist())
}

func (b *BotRunner) sendRejectionsToUsers(prefStore preferences.PreferenceStore, rejections []domain.Rejection, whitelist []int64) error {
	prefs, err := prefStore.List()
	if err != nil {
		return fmt.Errorf("failed to list notification preferences: %w", err)
	}

	allowed := toAllowedRecipientsMap(whitelist)
	var sendErrors []error
	recipients := 0
	var tgBot Notifier

	for _, pref := range prefs {
		if !pref.Enabled || (!pref.Authorized && !isAllowedRecipient(pref.ChatID, allowed)) {
			continue
		}

		selected := filterRejectionsBySections(rejections, pref.Sections())
		if len(selected) == 0 {
			continue
		}

		if tgBot == nil {
			createdBot, err := b.ensureNotifier()
			if err != nil {
				sendErrors = append(sendErrors, fmt.Errorf("failed to create telegram notifier: %w", err))
				continue
			}
			tgBot = createdBot
		}

		if err := tgBot.SendRejectionsNotification(pref.ChatID, selected); err != nil {
			sendErrors = append(sendErrors, fmt.Errorf("failed to send scheduled notification: %w", err))
			continue
		}

		recipients++
	}

	slog.Info("scheduled notification fan-out complete", "recipients", recipients, "rejections_count", len(rejections))
	return errors.Join(sendErrors...)
}

func toAllowedRecipientsMap(whitelist []int64) map[int64]struct{} {
	allowed := make(map[int64]struct{}, len(whitelist))
	for _, chatID := range whitelist {
		allowed[chatID] = struct{}{}
	}
	return allowed
}

func isAllowedRecipient(chatID int64, whitelist map[int64]struct{}) bool {
	if len(whitelist) == 0 {
		return true
	}
	_, ok := whitelist[chatID]
	return ok
}

func filterRejectionsBySections(rejections []domain.Rejection, sections []string) []domain.Rejection {
	selected := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		selected[section] = struct{}{}
	}

	filtered := make([]domain.Rejection, 0, len(rejections))
	for _, rejection := range rejections {
		if _, ok := selected[rejection.Section]; ok {
			filtered = append(filtered, rejection)
		}
	}
	return filtered
}

func (b *BotRunner) sendNoRejectionsMessage(chatID int64, message string) error {
	logger := slog.Default()
	logger.Info("sending no rejections message")

	if existing := b.currentNotifier(); existing != nil {
		logger.Info("using existing notifier")
		return existing.SendNoRejectionsMessage(chatID, message)
	}

	tgBot, err := b.newTransientNotifier(chatID)
	if err != nil {
		logger.Error(telegramBotTokenNotConfigured)
		if b.telemetryClient != nil && err.Error() != telegramBotTokenNotConfigured {
			b.telemetryClient.CaptureError(err, map[string]any{
				"phase": "create_temp_notifier",
			})
		}
		return err
	}

	return tgBot.SendNoRejectionsMessage(chatID, message)
}

func (b *BotRunner) sendRejectionsNotification(chatID int64, rejections []domain.Rejection) error {
	if existing := b.currentNotifier(); existing != nil {
		return existing.SendRejectionsNotification(chatID, rejections)
	}

	tgBot, err := b.newTransientNotifier(chatID)
	if err != nil {
		return err
	}

	return tgBot.SendRejectionsNotification(chatID, rejections)
}

func (b *BotRunner) ensurePreferenceStore() (preferences.PreferenceStore, func(), error) {
	if b.prefStore != nil {
		return b.prefStore, noOpPreferenceStoreClose, nil
	}
	if b.cfg == nil || b.cfg.DatabaseURL == "" {
		return nil, noOpPreferenceStoreClose, fmt.Errorf("DATABASE_URL not configured")
	}

	store, err := newPreferenceStoreFromDatabaseURL(b.cfg.DatabaseURL)
	if err != nil {
		return nil, noOpPreferenceStoreClose, err
	}
	prefStore := store

	closeStore := noOpPreferenceStoreClose
	if closer, ok := any(prefStore).(interface{ Close() error }); ok {
		closeStore = func() { _ = closer.Close() }
	}

	return prefStore, closeStore, nil
}

func (b *BotRunner) ensureNotifier() (Notifier, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.notifier != nil {
		return b.notifier, nil
	}

	token := b.getBotToken()
	if token == "" {
		return nil, errors.New(telegramBotTokenNotConfigured)
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

func (b *BotRunner) currentNotifier() Notifier {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.notifier
}

func (b *BotRunner) newTransientNotifier(chatID int64) (Notifier, error) {
	token := b.getBotToken()
	if token == "" {
		return nil, errors.New(telegramBotTokenNotConfigured)
	}

	whitelist := append(b.getWhitelist(), chatID)
	slog.Default().Info("creating temporary notifier")

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
		for idStr := range strings.SplitSeq(whitelistEnv, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				continue
			}
			whitelist = append(whitelist, id)
		}
	}
	return whitelist
}
