// Package preferences provides user notification preference management.
package preferences

import "time"

// UserPreference represents a user's notification preferences.
type UserPreference struct {
	ChatID    int64     `json:"chat_id"`
	Username  string    `json:"username"`
	Sections  []string  `json:"sections"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
