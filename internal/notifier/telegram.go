package notifier

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/preferences"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// AllSections lists all configurable sections for notifications.
var AllSections = []string{
	"Partes Mecânicas",
	"Campo",
	"Testemunho Público",
	"Reunião Meio de Semana",
}

type CheckNowCallback func(ctx context.Context, chatID int64) error

// TelegramNotifier sends notifications via Telegram Bot.
type TelegramNotifier struct {
	bot              *bot.Bot
	chatID           int64
	whitelist        []int64
	prefManager      *preferences.PreferenceManager
	cancelFunc       context.CancelFunc
	mu               sync.Mutex
	checkNowCallback CheckNowCallback
}

// NewTelegramNotifier creates a new Telegram notifier.
func NewTelegramNotifier(token string, chatID int64, whitelist []int64) (*TelegramNotifier, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	if chatID == 0 {
		return nil, fmt.Errorf("telegram chat ID is required")
	}

	b, err := bot.New(token, bot.WithDefaultHandler(func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		// Silent handler - do nothing to suppress [TGBOT] [UPDATE] logs
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramNotifier{
		bot:       b,
		chatID:    chatID,
		whitelist: whitelist,
	}, nil
}

// IsAuthorized checks if the chatID is in the whitelist.
func (t *TelegramNotifier) IsAuthorized(chatID int64) bool {
	if len(t.whitelist) == 0 {
		return true
	}
	for _, id := range t.whitelist {
		if id == chatID {
			return true
		}
	}
	return false
}

func (t *TelegramNotifier) SendNoRejectionsMessage(chatID int64, message string) error {
	if !t.IsAuthorized(chatID) {
		return fmt.Errorf("unauthorized chat ID: %d", chatID)
	}

	_, err := t.bot.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      message,
		ParseMode: models.ParseModeHTML,
	})

	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	return nil
}

// SendRejectionsNotification sends a notification about rejections to a specific chat ID.
func (t *TelegramNotifier) SendRejectionsNotification(chatID int64, rejections []domain.Rejeicao) error {
	if len(rejections) == 0 {
		return nil
	}

	if !t.IsAuthorized(chatID) {
		return fmt.Errorf("unauthorized chat ID: %d", chatID)
	}

	// Build message (using HTML instead of Markdown to avoid escaping issues)
	var msg strings.Builder
	msg.WriteString("<b>❌ Rejections Detected in Hourglass</b>\n\n")
	msg.WriteString(fmt.Sprintf("<b>%d</b> assignment(s) rejected:\n\n", len(rejections)))

	for i, r := range rejections {
		msg.WriteString(fmt.Sprintf("<b>Rejection #%d:</b>\n", i+1))
		msg.WriteString(fmt.Sprintf("👤 <b>Who:</b> %s\n", r.Quem))
		msg.WriteString(fmt.Sprintf("📋 <b>Section:</b> %s\n", r.Secao))
		msg.WriteString(fmt.Sprintf("📝 <b>Assignment:</b> %s\n", r.OQue))
		msg.WriteString(fmt.Sprintf("📅 <b>Date:</b> %s\n\n", r.PraQuando))
	}

	// Send message
	_, err := t.bot.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      msg.String(),
		ParseMode: models.ParseModeHTML,
	})

	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	return nil
}

// IsConfigured checks if the notifier is properly configured.
func (t *TelegramNotifier) IsConfigured() bool {
	return t != nil && t.bot != nil && t.chatID != 0
}

func (t *TelegramNotifier) SetCheckNowCallback(callback CheckNowCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.checkNowCallback = callback
}

// StartBot starts the bot in listener mode with interactive handlers.
func (t *TelegramNotifier) StartBot(ctx context.Context, prefManager *preferences.PreferenceManager) error {
	if prefManager == nil {
		return fmt.Errorf("preference manager is required")
	}

	t.mu.Lock()
	t.prefManager = prefManager
	t.mu.Unlock()

	// Register command handlers
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, t.handleStart)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/configure", bot.MatchTypeExact, t.handleConfig)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/status", bot.MatchTypeExact, t.handleStatus)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, t.handleHelp)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/checknow", bot.MatchTypeExact, t.handleCheckNow)

	// Register callback handlers for inline keyboard
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "section_", bot.MatchTypePrefix, t.handleSectionToggle)
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "save_config", bot.MatchTypeExact, t.handleSave)
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "cancel_config", bot.MatchTypeExact, t.handleCancel)

	commands := []models.BotCommand{
		{Command: "start", Description: "Welcome message"},
		{Command: "configure", Description: "Configure notification sections"},
		{Command: "status", Description: "View current preferences"},
		{Command: "help", Description: "Show available commands"},
		{Command: "checknow", Description: "Immediate check"},
	}

	_, err := t.bot.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: commands,
	})
	if err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	botCtx, cancel := context.WithCancel(ctx)
	t.mu.Lock()
	t.cancelFunc = cancel
	t.mu.Unlock()

	go t.bot.Start(botCtx)

	return nil
}

// StopBot stops the bot gracefully.
func (t *TelegramNotifier) StopBot() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancelFunc != nil {
		t.cancelFunc()
		t.cancelFunc = nil
	}

	return nil
}

// handleStart handles the /start command.
func (t *TelegramNotifier) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	username := ""
	if update.Message.From != nil {
		username = update.Message.From.Username
	}

	// Save discovered chat (user who messaged the bot) - separate from whitelist
	if t.prefManager != nil {
		_ = t.prefManager.RecordDiscoveredChat(chatID, username)
	}

	if !t.IsAuthorized(chatID) {
		text := "🤖 <b>Welcome to Hourglass RPA Bot!</b>\n\n" +
			"You can interact with the bot, but <b>you are not authorized to receive notifications</b>.\n\n" +
			"📧 To receive rejection notifications, contact the administrator " +
			"and request that your Chat ID be added to the whitelist.\n\n" +
			"Your Chat ID: <code>" + fmt.Sprintf("%d", chatID) + "</code>"

		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	// Ensure user preferences exist
	if t.prefManager != nil {
		_, _ = t.prefManager.GetOrCreate(chatID, username)
	}

	text := "🤖 <b>Welcome to Hourglass RPA Bot!</b>\n\n" +
		"Use /configure to choose which sections you want to receive notifications for.\n" +
		"Use /status to view your current preferences.\n" +
		"Use /help to see all available commands."

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

// handleConfig handles the /configure command.
func (t *TelegramNotifier) handleConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	username := update.Message.From.Username

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			//nolint:misspell // Portuguese text
			Text:      "❌ You are not authorized to use this command. Contact the administrator.",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	if t.prefManager == nil {
		return
	}

	pref, err := t.prefManager.GetOrCreate(chatID, username)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      "❌ Error loading preferences. Please try again.",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "⚙️ <b>Choose sections to receive notifications for:</b>",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: t.buildConfigKeyboard(pref),
	})
}

// handleStatus handles the /status command.
func (t *TelegramNotifier) handleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	username := update.Message.From.Username

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			//nolint:misspell // Portuguese text
			Text:      "❌ You are not authorized to use this command. Contact the administrator.",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	if t.prefManager == nil {
		return
	}

	pref, err := t.prefManager.GetOrCreate(chatID, username)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      "❌ Error loading preferences. Please try again.",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	var msg strings.Builder
	msg.WriteString("📊 <b>Your preferences:</b>\n\n")

	for _, section := range AllSections {
		if containsSection(pref.Sections(), section) {
			msg.WriteString(fmt.Sprintf("✅ %s\n", section))
		} else {
			msg.WriteString(fmt.Sprintf("❌ %s\n", section))
		}
	}

	if pref.Enabled {
		msg.WriteString("\n🔔 Notifications: <b>Enabled</b>")
	} else {
		msg.WriteString("\n🔕 Notifications: <b>Disabled</b>")
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      msg.String(),
		ParseMode: models.ParseModeHTML,
	})
}

// handleHelp handles the /help command.
func (t *TelegramNotifier) handleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	text := "📖 <b>Available commands:</b>\n\n" +
		"/start - Welcome message\n" +
		"/configure - Configure notification sections\n" +
		"/status - View current preferences\n" +
		"/help - Show this message\n" +
		"/checknow - Immediate check"

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

// handleCheckNow handles the /checknow command (admin only).
func (t *TelegramNotifier) handleCheckNow(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.IsAuthorized(chatID) {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      "⛔ You do not have permission to execute this command.",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      "🔄 Verificação imediata solicitada. Processando...",
		ParseMode: models.ParseModeHTML,
	})

	t.mu.Lock()
	callback := t.checkNowCallback
	t.mu.Unlock()

	if callback == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      "⚠️ Manual check not available in bot mode. Use scheduler mode.",
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	go func() {
		if err := callback(ctx, chatID); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      fmt.Sprintf("❌ Erro na verificação: %v", err),
				ParseMode: models.ParseModeHTML,
			})
		}
	}()
}

// handleSectionToggle handles section toggle callback queries.
func (t *TelegramNotifier) handleSectionToggle(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            "You are not authorized. Contact the administrator.",
		})
		return
	}

	// CRITICAL: Always answer callback first
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	if t.prefManager == nil {
		return
	}

	username := update.CallbackQuery.From.Username

	// Extract section name from callback data ("section_Campo" -> "Campo")
	section := strings.TrimPrefix(update.CallbackQuery.Data, "section_")

	pref, err := t.prefManager.GetOrCreate(chatID, username)
	if err != nil {
		return
	}

	// Toggle the section
	sections := pref.Sections()
	if containsSection(sections, section) {
		sections = removeSection(sections, section)
	} else {
		sections = append(sections, section)
	}
	pref.SetSections(sections)

	// Save toggled state temporarily via UpdateSections
	_ = t.prefManager.UpdateSections(chatID, sections)

	// Update the inline keyboard
	if update.CallbackQuery.Message.Message != nil {
		b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			ReplyMarkup: t.buildConfigKeyboard(pref),
		})
	}
}

// handleSave handles the save configuration callback query.
func (t *TelegramNotifier) handleSave(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            "You are not authorized. Contact the administrator.",
		})
		return
	}

	// CRITICAL: Always answer callback first
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	if t.prefManager == nil {
		return
	}

	username := update.CallbackQuery.From.Username

	pref, err := t.prefManager.GetOrCreate(chatID, username)
	if err != nil {
		return
	}

	// Preferences are already saved by toggle handler; confirm to user
	var msg strings.Builder
	msg.WriteString("✅ <b>Preferences saved!</b>\n\n")

	if len(pref.Sections()) == 0 {
		msg.WriteString("No sections selected. You will not receive notifications.")
	} else {
		msg.WriteString("Selected sections:\n")
		for _, s := range pref.Sections() {
			msg.WriteString(fmt.Sprintf("• %s\n", s))
		}
	}

	// Replace the keyboard message with confirmation
	if update.CallbackQuery.Message.Message != nil {
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      msg.String(),
			ParseMode: models.ParseModeHTML,
		})
	}
}

// handleCancel handles the cancel configuration callback query.
func (t *TelegramNotifier) handleCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            "You are not authorized. Contact the administrator.",
		})
		return
	}

	// CRITICAL: Always answer callback first
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	// Replace the keyboard message with cancellation
	if update.CallbackQuery.Message.Message != nil {
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      "❌ Configuration cancelled.",
			ParseMode: models.ParseModeHTML,
		})
	}
}

// buildConfigKeyboard builds an inline keyboard for section configuration.
func (t *TelegramNotifier) buildConfigKeyboard(pref *preferences.UserPreference) models.ReplyMarkup {
	var rows [][]models.InlineKeyboardButton

	for _, section := range AllSections {
		var label string
		if containsSection(pref.Sections(), section) {
			label = "✅ " + section
		} else {
			label = "❌ " + section
		}

		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: "section_" + section},
		})
	}

	// Add Save and Cancel buttons
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "💾 Save", CallbackData: "save_config"},
		{Text: "🚫 Cancel", CallbackData: "cancel_config"},
	})

	return &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

// containsSection checks if a section is in the list.
func containsSection(sections []string, section string) bool {
	for _, s := range sections {
		if s == section {
			return true
		}
	}
	return false
}

// removeSection removes a section from the list.
func removeSection(sections []string, section string) []string {
	result := make([]string, 0, len(sections))
	for _, s := range sections {
		if s != section {
			result = append(result, s)
		}
	}
	return result
}
