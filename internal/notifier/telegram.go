package notifier

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hourglass-rejections-rpa/internal/domain"
)

// TelegramNotifier sends notifications via Telegram Bot.
type TelegramNotifier struct {
	bot       *bot.Bot
	chatID    int64
	whitelist []int64
}

// NewTelegramNotifier creates a new Telegram notifier.
func NewTelegramNotifier(token string, chatID int64, whitelist []int64) (*TelegramNotifier, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	if chatID == 0 {
		return nil, fmt.Errorf("telegram chat ID is required")
	}

	b, err := bot.New(token)
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

// SendRejectionsNotification sends a notification about rejections.
func (t *TelegramNotifier) SendRejectionsNotification(rejections []domain.Rejeicao) error {
	if len(rejections) == 0 {
		return nil
	}

	if !t.IsAuthorized(t.chatID) {
		return fmt.Errorf("unauthorized chat ID: %d", t.chatID)
	}

	// Build message (using HTML instead of Markdown to avoid escaping issues)
	var msg strings.Builder
	msg.WriteString("<b>❌ Rejeições Detectadas no Hourglass</b>\n\n")
	msg.WriteString(fmt.Sprintf("Foram detectadas <b>%d</b> designação(ões) recusada(s):\n\n", len(rejections)))

	for i, r := range rejections {
		msg.WriteString(fmt.Sprintf("<b>Rejeição #%d:</b>\n", i+1))
		msg.WriteString(fmt.Sprintf("👤 <b>Quem:</b> %s\n", r.Quem))
		msg.WriteString(fmt.Sprintf("📋 <b>Seção:</b> %s\n", r.Secao))
		msg.WriteString(fmt.Sprintf("📝 <b>Designação:</b> %s\n", r.OQue))
		msg.WriteString(fmt.Sprintf("📅 <b>Data:</b> %s\n\n", r.PraQuando))
	}

	// Send message
	_, err := t.bot.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID:    t.chatID,
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
