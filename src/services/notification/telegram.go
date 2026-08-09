package notifier

import (
	"context"
	"fmt"
	"html"
	"slices"
	"strings"
	"sync"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/i18n"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const botStatsDateLayout = "2006-01-02"

// CheckNowCallback is a callback function for triggering immediate checks.
type CheckNowCallback func(ctx context.Context, chatID int64) error

// botNewFunc is a package-level variable to allow testing the constructor.
var botNewFunc = bot.New

func noopDefaultUpdateHandler(_ context.Context, _ *bot.Bot, _ *models.Update) {}

// TelegramNotifier sends notifications via Telegram Bot.
type rateLimiter struct {
	mu       sync.RWMutex
	attempts map[int64][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		attempts: make(map[int64][]time.Time),
	}
}

func (rl *rateLimiter) Allow(chatID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	var valid []time.Time
	for _, t := range rl.attempts[chatID] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= 30 {
		rl.attempts[chatID] = valid
		return false
	}

	valid = append(valid, now)
	rl.attempts[chatID] = valid
	return true
}

type TelegramNotifier struct {
	bot              *bot.Bot
	chatID           int64
	whitelist        []int64
	prefManager      *preferences.PreferenceManager
	cancelFunc       context.CancelFunc
	mu               sync.Mutex
	checkNowCallback CheckNowCallback
	rateLimiter      *rateLimiter
	stats            *botStats
}

type botStats struct {
	mu              sync.RWMutex
	totalChecks     int
	lastResetDate   string
	rejectionsToday int
}

func newBotStats() *botStats {
	return &botStats{lastResetDate: time.Now().Format(botStatsDateLayout)}
}

func (s *botStats) recordCheck(rejectionsFound int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format(botStatsDateLayout)
	if s.lastResetDate != today {
		s.lastResetDate = today
		s.rejectionsToday = 0
	}

	s.totalChecks++
	if rejectionsFound > 0 {
		s.rejectionsToday += rejectionsFound
	}
}

func (s *botStats) snapshot() (int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalChecks, s.rejectionsToday
}

// formatTelegramField formats a field for Telegram HTML messages with proper escaping.
func formatTelegramField(emoji, label, value string) string {
	return fmt.Sprintf("%s <b>%s:</b> %s\n", emoji, label, html.EscapeString(value))
}

// NewTelegramNotifier creates a new Telegram notifier.
func NewTelegramNotifier(token string, chatID int64, whitelist []int64) (*TelegramNotifier, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	if chatID == 0 {
		return nil, fmt.Errorf("telegram chat ID is required")
	}

	b, err := botNewFunc(token, bot.WithDefaultHandler(noopDefaultUpdateHandler))
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramNotifier{
		bot:         b,
		chatID:      chatID,
		whitelist:   whitelist,
		rateLimiter: newRateLimiter(),
		stats:       newBotStats(),
	}, nil
}

// IsAuthorized checks if the chatID is in the whitelist.
func (t *TelegramNotifier) IsAuthorized(chatID int64) bool {
	if len(t.whitelist) == 0 {
		return true
	}
	return slices.Contains(t.whitelist, chatID)
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

	if t.stats != nil {
		t.stats.recordCheck(0)
	}

	return nil
}

// SendRejectionsNotification sends a notification about rejections to a specific chat ID.
func (t *TelegramNotifier) SendRejectionsNotification(chatID int64, rejections []domain.Rejection) error {
	if len(rejections) == 0 {
		return nil
	}

	if !t.IsAuthorized(chatID) {
		return fmt.Errorf("unauthorized chat ID: %d", chatID)
	}

	lang := t.getUserLanguage(chatID)

	rejectionsList := make([]map[string]any, 0, len(rejections))
	for i, r := range rejections {
		rejectionsList = append(rejectionsList, map[string]any{
			"Number":  i + 1,
			"Who":     html.EscapeString(r.Who),
			"Section": html.EscapeString(translateSectionName(lang, r.Section)),
			"What":    html.EscapeString(translateAssignmentType(lang, r.What)),
			"When":    html.EscapeString(i18n.FormatDate(r.When, lang)),
		})
	}

	msg := i18n.Localize(lang, "rejections_detected", map[string]any{
		"Count":      len(rejections),
		"Rejections": rejectionsList,
	})

	_, err := t.bot.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      msg,
		ParseMode: models.ParseModeHTML,
	})

	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	if t.stats != nil {
		t.stats.recordCheck(len(rejections))
	}

	return nil
}

// IsConfigured checks if the notifier is properly configured.
func (t *TelegramNotifier) IsConfigured() bool {
	return t != nil && t.bot != nil && t.chatID != 0
}

// getUserLanguage returns the user's language preference
func (t *TelegramNotifier) getUserLanguage(chatID int64) string {
	if t.prefManager == nil {
		return "en"
	}
	return t.prefManager.GetLanguage(chatID)
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
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/stats", bot.MatchTypeExact, t.handleStats)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/whoami", bot.MatchTypeExact, t.handleWhoAmI)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/language", bot.MatchTypeExact, t.handleLanguage)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, t.handleHelp)
	t.bot.RegisterHandler(bot.HandlerTypeMessageText, "/checknow", bot.MatchTypeExact, t.handleCheckNow)

	// Register callback handlers for inline keyboard
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "section_", bot.MatchTypePrefix, t.handleSectionToggle)
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "save_config", bot.MatchTypeExact, t.handleSave)
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "cancel_config", bot.MatchTypeExact, t.handleCancel)
	t.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, "lang_", bot.MatchTypePrefix, t.handleLanguageSelect)

	commands := []models.BotCommand{
		{Command: "start", Description: "Welcome message"},
		{Command: "configure", Description: "Configure notification sections"},
		{Command: "status", Description: "View current preferences"},
		{Command: "stats", Description: "View bot statistics"},
		{Command: "whoami", Description: "View your account details"},
		{Command: "language", Description: "Change language"},
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

// checkRateLimit checks if the user has exceeded the rate limit.
func (t *TelegramNotifier) checkRateLimit(ctx context.Context, b *bot.Bot, chatID int64) bool {
	if t.rateLimiter == nil {
		return true
	}
	if !t.rateLimiter.Allow(chatID) {
		lang := t.getUserLanguage(chatID)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "rate_limit_exceeded", nil),
			ParseMode: models.ParseModeHTML,
		})
		return false
	}
	return true
}

func (t *TelegramNotifier) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}
	username := ""
	if update.Message.From != nil {
		username = update.Message.From.Username
	}

	// Save discovered chat (user who messaged the bot) - separate from whitelist
	if t.prefManager != nil {
		_ = t.prefManager.RecordDiscoveredChat(chatID, username)
	}

	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		text := i18n.Localize(lang, "welcome_unauthorized", map[string]any{
			"ChatID": fmt.Sprintf("%d", chatID),
		})

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

	text := i18n.Localize(lang, "welcome", nil)

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func (t *TelegramNotifier) handleConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}
	username := update.Message.From.Username
	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "unauthorized_config", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	if t.prefManager == nil {
		return
	}

	pref, err := t.prefManager.GetOrCreate(chatID, username)
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "configure_error", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        i18n.Localize(lang, "choose_sections", nil),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: t.buildConfigKeyboard(pref, lang),
	})
}

func (t *TelegramNotifier) handleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	username := update.Message.From.Username
	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "unauthorized", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	if t.prefManager == nil {
		return
	}

	pref, err := t.prefManager.GetOrCreate(chatID, username)
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "configure_error", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	sections := pref.Sections()
	sectionsList := make([]map[string]string, 0, len(domain.AllSections))
	for _, section := range domain.AllSections {
		status := "disabled"
		if containsSection(sections, section) {
			status = "enabled"
		}
		sectionsList = append(sectionsList, map[string]string{
			"Name":   section,
			"Status": status,
		})
	}

	notificationStatus := "disabled"
	if pref.Enabled {
		notificationStatus = "enabled"
	}

	msg := i18n.Localize(lang, "your_preferences", map[string]any{
		"Sections":           sectionsList,
		"NotificationStatus": notificationStatus,
	})

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      msg,
		ParseMode: models.ParseModeHTML,
	})
}

func (t *TelegramNotifier) handleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)
	text := i18n.Localize(lang, "help_commands", nil)

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func (t *TelegramNotifier) handleStats(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)

	totalUsers, err := t.totalPreferenceUsers()
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "configure_error", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "unauthorized", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	totalChecks, rejectionsToday := 0, 0
	if t.stats != nil {
		totalChecks, rejectionsToday = t.stats.snapshot()
	}

	msg := i18n.Localize(lang, "stats_overview", map[string]any{
		"TotalUsers":      totalUsers,
		"TotalChecks":     totalChecks,
		"RejectionsToday": rejectionsToday,
	})

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      msg,
		ParseMode: models.ParseModeHTML,
	})
}

func (t *TelegramNotifier) totalPreferenceUsers() (int, error) {
	if t.prefManager == nil {
		return 0, nil
	}

	prefs, err := t.prefManager.List()
	if err != nil {
		return 0, err
	}

	return len(prefs), nil
}

func translateSectionName(lang, sectionName string) string {
	sectionKey := ""
	switch sectionName {
	case "Partes Mecânicas", "Mechanical Parts":
		sectionKey = "section_partes_mecanicas"
	case "Campo", "Field Ministry":
		sectionKey = "section_campo"
	case "Testemunho Público", "Public Witnessing":
		sectionKey = "section_testemunho_publico"
	case "Reunião Meio de Semana", "Midweek Meeting":
		sectionKey = "section_reuniao_meio_semana"
	default:
		return sectionName
	}
	return i18n.Localize(lang, sectionKey, nil)
}

func translateAssignmentType(lang, assignmentType string) string {
	assignmentKey := ""
	switch assignmentType {
	case "Audio/Video & Indicators":
		assignmentKey = "assignment_audio_video_indicators"
	case "Video":
		assignmentKey = "assignment_video"
	case "Console":
		assignmentKey = "assignment_console"
	case "Microphone":
		assignmentKey = "assignment_microphone"
	case "Attendant":
		assignmentKey = "assignment_attendant"
	case "Public Witnessing":
		assignmentKey = "assignment_public_witnessing"
	case "Field Ministry Meeting":
		assignmentKey = "assignment_field_ministry_meeting"
	case "Bible Reading":
		assignmentKey = "assignment_bible_reading"
	case "Speaker/Chairman":
		assignmentKey = "assignment_speaker_chairman"
	case "Student":
		assignmentKey = "assignment_student"
	case "Assistant":
		assignmentKey = "assignment_assistant"
	case "Special Assignment":
		assignmentKey = "assignment_special_assignment"
	case "Other Assignment":
		assignmentKey = "assignment_other_assignment"
	default:
		return assignmentType
	}
	return i18n.Localize(lang, assignmentKey, nil)
}

func (t *TelegramNotifier) handleWhoAmI(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	authorizedStatus, languagePreference, sectionsDisplay := t.buildWhoAmIData(chatID)
	lang := t.getUserLanguage(chatID)

	msg := i18n.Localize(lang, "whoami_info", map[string]any{
		"ChatID":     chatID,
		"Authorized": authorizedStatus,
		"Language":   languagePreference,
		"Sections":   sectionsDisplay,
	})

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      msg,
		ParseMode: models.ParseModeHTML,
	})
}

func (t *TelegramNotifier) buildWhoAmIData(chatID int64) (string, string, string) {
	lang := t.getUserLanguage(chatID)
	authorizedStatus := i18n.Localize(lang, "status_no", nil)
	if t.IsAuthorized(chatID) {
		authorizedStatus = i18n.Localize(lang, "status_yes", nil)
	}

	languagePreference := lang
	sectionsDisplay := i18n.Localize(lang, "whoami_no_sections", nil)

	pref, err := t.getPreference(chatID)
	if err != nil || pref == nil {
		return authorizedStatus, languagePreference, sectionsDisplay
	}

	if pref.Language != "" {
		languagePreference = pref.Language
	}

	sections := pref.Sections()
	if len(sections) == 0 {
		return authorizedStatus, languagePreference, sectionsDisplay
	}

	translatedSections := make([]string, len(sections))
	for i, section := range sections {
		translatedSections[i] = translateSectionName(languagePreference, section)
	}

	sectionsDisplay = strings.Join(translatedSections, ", ")
	return authorizedStatus, languagePreference, sectionsDisplay
}

func (t *TelegramNotifier) getPreference(chatID int64) (*preferences.UserPreference, error) {
	if t.prefManager == nil {
		return nil, nil
	}

	return t.prefManager.Get(chatID)
}

func (t *TelegramNotifier) handleCheckNow(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	lang := t.getUserLanguage(chatID)

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "unauthorized", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      i18n.Localize(lang, "check_now_requested", nil),
		ParseMode: models.ParseModeHTML,
	})

	t.mu.Lock()
	callback := t.checkNowCallback
	t.mu.Unlock()

	if callback == nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "check_now_unavailable", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	go func() {
		if err := callback(ctx, chatID); err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      i18n.Localize(lang, "verification_error", map[string]any{"Error": html.EscapeString(err.Error())}),
				ParseMode: models.ParseModeHTML,
			})
			if t.stats != nil {
				t.stats.recordCheck(0)
			}
		}
	}()
}

func (t *TelegramNotifier) handleSectionToggle(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	if update.CallbackQuery.Message.Message == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            i18n.Localize(lang, "unauthorized", nil),
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
		_, _ = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			ReplyMarkup: t.buildConfigKeyboard(pref, lang),
		})
	}
}

func (t *TelegramNotifier) handleSave(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            i18n.Localize(lang, "unauthorized", nil),
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
	sectionsList := pref.Sections()

	msg := i18n.Localize(lang, "preferences_saved", map[string]any{
		"Sections": sectionsList,
	})

	// Replace the keyboard message with confirmation
	if update.CallbackQuery.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      msg,
			ParseMode: models.ParseModeHTML,
		})
	}
}

func (t *TelegramNotifier) handleCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            i18n.Localize(lang, "unauthorized", nil),
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
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      i18n.Localize(lang, "configuration_cancelled", nil),
			ParseMode: models.ParseModeHTML,
		})
	}
}

func (t *TelegramNotifier) handleLanguage(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      i18n.Localize(lang, "unauthorized", nil),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🇺🇸 " + i18n.Localize(lang, "language_english", nil), CallbackData: "lang_en"},
			},
			{
				{Text: "🇧🇷 " + i18n.Localize(lang, "language_portuguese", nil), CallbackData: "lang_pt-BR"},
			},
			{
				{Text: "🇪🇸 " + i18n.Localize(lang, "language_spanish", nil), CallbackData: "lang_es"},
			},
			{
				{Text: "🇫🇷 " + i18n.Localize(lang, "language_french", nil), CallbackData: "lang_fr"},
			},
		},
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        i18n.Localize(lang, "language_select", nil),
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
}

func (t *TelegramNotifier) handleLanguageSelect(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	chatID := update.CallbackQuery.From.ID

	if !t.checkRateLimit(ctx, b, chatID) {
		return
	}

	lang := t.getUserLanguage(chatID)

	if !t.IsAuthorized(chatID) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			ShowAlert:       true,
			Text:            i18n.Localize(lang, "unauthorized", nil),
		})
		return
	}

	selectedLang := strings.TrimPrefix(update.CallbackQuery.Data, "lang_")

	if t.prefManager != nil {
		_ = t.prefManager.UpdateLanguage(chatID, selectedLang)
	}

	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	if update.CallbackQuery.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      i18n.Localize(selectedLang, "language_changed", nil),
			ParseMode: models.ParseModeHTML,
		})
	}
}

// buildConfigKeyboard builds an inline keyboard for section configuration.
func (t *TelegramNotifier) buildConfigKeyboard(pref *preferences.UserPreference, lang string) models.ReplyMarkup {
	var rows [][]models.InlineKeyboardButton

	for _, section := range domain.AllSections {
		translatedSection := translateSectionName(lang, section)
		var label string
		if containsSection(pref.Sections(), section) {
			label = "✅ " + translatedSection
		} else {
			label = "❌ " + translatedSection
		}

		rows = append(rows, []models.InlineKeyboardButton{
			{Text: label, CallbackData: "section_" + section},
		})
	}

	// Add Save and Cancel buttons
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "💾 " + i18n.Localize(lang, "btn_save", nil), CallbackData: "save_config"},
		{Text: "🚫 " + i18n.Localize(lang, "btn_cancel", nil), CallbackData: "cancel_config"},
	})

	return &models.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

// containsSection checks if a section is in the list.
func containsSection(sections []string, section string) bool {
	return slices.Contains(sections, section)
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
