//go:build verify

package notifier

import (
	"fmt"

	"github.com/go-telegram/bot"
)

// NewTelegramNotifierForVerify builds a TelegramNotifier pointed at a mock Bot API.
// Production builds omit this constructor; only the verify harness uses -tags verify.
func NewTelegramNotifierForVerify(token string, chatID int64, whitelist []int64, serverURL string) (*TelegramNotifier, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if chatID == 0 {
		return nil, fmt.Errorf("telegram chat ID is required")
	}
	if serverURL == "" {
		return nil, fmt.Errorf("telegram server URL is required")
	}

	b, err := botNewFunc(
		token,
		bot.WithDefaultHandler(noopDefaultUpdateHandler),
		bot.WithServerURL(serverURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramNotifier{
		bot:         b,
		chatID:      chatID,
		adminID:     chatID,
		whitelist:   whitelist,
		rateLimiter: newRateLimiter(),
		stats:       newBotStats(),
	}, nil
}
