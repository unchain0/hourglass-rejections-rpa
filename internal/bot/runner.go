package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"hourglass-rejections-rpa/internal/api"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/notifier"
	"hourglass-rejections-rpa/internal/preferences"
	"hourglass-rejections-rpa/internal/sentry"
	"hourglass-rejections-rpa/internal/storage"
)

type Analyzer interface {
	AnalyzeSection(section string) (*domain.JobResult, error)
}

type Notifier interface {
	StartBot(ctx context.Context, prefManager *preferences.PreferenceManager) error
	StopBot() error
	SetCheckNowCallback(callback notifier.CheckNowCallback)
	SendNoRejectionsMessage(chatID int64, message string) error
	SendRejectionsNotification(chatID int64, rejections []domain.Rejeicao) error
}

type BotRunner struct {
	cfg          *config.Config
	sentryClient *sentry.Client
	analyzer     Analyzer
	store        *storage.FileStorage
	mu           sync.RWMutex

	notifier  Notifier
	prefStore preferences.PreferenceStore
}

func New(cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) *BotRunner {
	return &BotRunner{
		cfg:          cfg,
		sentryClient: sentryClient,
		analyzer:     analyzer,
		store:        store,
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

func (b *BotRunner) Run(ctx context.Context) error {
	logger := slog.Default()

	var prefStore preferences.PreferenceStore
	if b.prefStore != nil {
		prefStore = b.prefStore
	} else {
		var err error
		prefStore, err = preferences.NewStore(b.cfg.SQLiteDBPath)
		if err != nil {
			if b.sentryClient != nil {
				b.sentryClient.CaptureError(err, map[string]interface{}{
					"phase":   "init_preference_store",
					"db_path": b.cfg.SQLiteDBPath,
				})
			}
			return fmt.Errorf("failed to initialize preference store: %w", err)
		}
		if closer, ok := prefStore.(interface{ Close() error }); ok {
			defer closer.Close()
		}
	}

	prefManager := preferences.NewPreferenceManager(prefStore)

	var tgBot Notifier
	if b.notifier != nil {
		tgBot = b.notifier
	} else {
		token := os.Getenv("TELEGRAM_BOT_TOKEN")
		if token == "" {
			return fmt.Errorf("TELEGRAM_BOT_TOKEN not configured")
		}

		whitelist := b.getWhitelist()
		var chatID int64
		if len(whitelist) > 0 {
			chatID = whitelist[0]
		}

		var err error
		tgBot, err = newTelegramNotifier(token, chatID, whitelist)
		if err != nil {
			if b.sentryClient != nil {
				b.sentryClient.CaptureError(err, map[string]interface{}{
					"phase":     "create_notifier",
					"chat_id":   chatID,
					"has_token": token != "",
				})
			}
			return fmt.Errorf("failed to create telegram notifier: %w", err)
		}
		b.notifier = tgBot
	}

	tgBot.SetCheckNowCallback(func(ctx context.Context, chatID int64) error {
		logger.Info("manual check triggered via bot", "chat_id", chatID)
		return b.runOnceForUser(ctx, prefManager, chatID)
	})

	logger.Info("starting telegram bot")

	if err := tgBot.StartBot(ctx, prefManager); err != nil {
		if b.sentryClient != nil {
			b.sentryClient.CaptureError(err, map[string]interface{}{
				"phase": "start_bot",
			})
		}
		return fmt.Errorf("failed to start bot: %w", err)
	}

	logger.Info("bot started successfully - send /start to your bot")

	<-ctx.Done()

	if err := tgBot.StopBot(); err != nil {
		logger.Error("error stopping bot", "error", err)
		if b.sentryClient != nil {
			b.sentryClient.CaptureError(err, map[string]interface{}{
				"phase": "stop_bot",
			})
		}
	}

	logger.Info("bot stopped")
	return nil
}

func (b *BotRunner) runOnceForUser(ctx context.Context, prefManager *preferences.PreferenceManager, targetChatID int64) error {
	logger := slog.Default()
	start := time.Now()

	pref, err := prefManager.Get(targetChatID)
	if err != nil {
		logger.Error("failed to get user preferences", "chat_id", targetChatID, "error", err)
		if b.sentryClient != nil {
			b.sentryClient.CaptureError(err, map[string]interface{}{
				"phase":   "get_user_preferences",
				"chat_id": targetChatID,
			})
		}
		return fmt.Errorf("failed to get user preferences: %w", err)
	}

	if pref == nil {
		logger.Error("user preferences not found", "chat_id", targetChatID)
		if b.sentryClient != nil {
			b.sentryClient.CaptureMessage("user preferences not found", "error")
		}
		return fmt.Errorf("user preferences not found")
	}

	userSections := pref.Sections()
	logger.Info("user preferences loaded", "chat_id", targetChatID, "sections", userSections, "sections_count", len(userSections))

	if len(userSections) == 0 {
		logger.Info("no sections configured, sending message", "chat_id", targetChatID)
		return b.sendNoRejectionsMessage(targetChatID, "Você não tem nenhuma seção configurada para monitoramento.")
	}

	var allRejections []domain.Rejeicao
	for _, section := range userSections {
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, stopping analysis", "chat_id", targetChatID)
			return ctx.Err()
		default:
		}

		sectionStart := time.Now()
		logger.Info("analyzing section for user", "section", section, "chat_id", targetChatID)

		result, err := b.analyzer.AnalyzeSection(section)
		if err != nil {
			logger.Error("failed to analyze section", "section", section, "error", err, "duration", time.Since(sectionStart))
			if b.sentryClient != nil {
				b.sentryClient.CaptureError(err, map[string]interface{}{
					"section":  section,
					"phase":    "analyze_section_for_user",
					"chat_id":  targetChatID,
					"duration": time.Since(sectionStart).String(),
				})
			}
			continue
		}

		if result.Error != nil {
			logger.Error("analysis returned error", "section", section, "error", result.Error, "duration", time.Since(sectionStart))
			if b.sentryClient != nil {
				b.sentryClient.CaptureError(result.Error, map[string]interface{}{
					"section":  section,
					"phase":    "analysis_result_for_user",
					"chat_id":  targetChatID,
					"total":    result.Total,
					"duration": time.Since(sectionStart).String(),
				})
			}
			continue
		}

		logger.Info("section analyzed", "section", section, "rejeicoes_count", len(result.Rejeicoes), "duration", time.Since(sectionStart))
		if len(result.Rejeicoes) > 0 {
			allRejections = append(allRejections, result.Rejeicoes...)
		}
	}

	totalDuration := time.Since(start)
	logger.Info("analysis complete", "chat_id", targetChatID, "total_rejeicoes", len(allRejections), "duration", totalDuration)

	if len(allRejections) == 0 {
		logger.Info("no rejections found, sending message", "chat_id", targetChatID)
		return b.sendNoRejectionsMessage(targetChatID, "✅ Nenhuma rejeição encontrada nas seções configuradas.")
	}

	logger.Info("sending rejections notification", "chat_id", targetChatID, "count", len(allRejections))
	return b.sendRejectionsNotification(targetChatID, allRejections)
}

func (b *BotRunner) sendNoRejectionsMessage(chatID int64, message string) error {
	logger := slog.Default()
	logger.Info("sending no rejections message", "chat_id", chatID, "message", message)

	if b.notifier != nil {
		logger.Info("using existing notifier")
		return b.notifier.SendNoRejectionsMessage(chatID, message)
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		logger.Error("TELEGRAM_BOT_TOKEN not configured")
		return fmt.Errorf("TELEGRAM_BOT_TOKEN not configured")
	}

	whitelist := b.getWhitelist()
	whitelist = append(whitelist, chatID)
	logger.Info("creating temporary notifier", "chat_id", chatID, "whitelist", whitelist)

	tgBot, err := newTelegramNotifier(token, chatID, whitelist)
	if err != nil {
		logger.Error("failed to create telegram notifier", "error", err)
		if b.sentryClient != nil {
			b.sentryClient.CaptureError(err, map[string]interface{}{
				"phase":   "create_temp_notifier",
				"chat_id": chatID,
			})
		}
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	return tgBot.SendNoRejectionsMessage(chatID, message)
}

func (b *BotRunner) sendRejectionsNotification(chatID int64, rejections []domain.Rejeicao) error {
	if b.notifier != nil {
		return b.notifier.SendRejectionsNotification(chatID, rejections)
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN not configured")
	}

	whitelist := b.getWhitelist()
	whitelist = append(whitelist, chatID)
	tgBot, err := newTelegramNotifier(token, chatID, whitelist)
	if err != nil {
		return fmt.Errorf("failed to create telegram notifier: %w", err)
	}

	return tgBot.SendRejectionsNotification(chatID, rejections)
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
