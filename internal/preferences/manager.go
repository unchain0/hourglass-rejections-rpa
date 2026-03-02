package preferences

import (
	"fmt"
	"time"
)

// PreferenceManager provides high-level operations on user preferences.
type PreferenceManager struct {
	store PreferenceStore
}

// NewPreferenceManager creates a new PreferenceManager with the given store.
func NewPreferenceManager(store PreferenceStore) *PreferenceManager {
	return &PreferenceManager{
		store: store,
	}
}

// GetOrCreate retrieves an existing preference or creates a new one with defaults.
func (pm *PreferenceManager) GetOrCreate(chatID int64, username string) (*UserPreference, error) {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get preference: %w", err)
	}

	if pref != nil {
		return pref, nil
	}

	now := time.Now().UTC()
	newPref := &UserPreference{
		ChatID:    chatID,
		Username:  username,
		Sections:  []string{},
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := pm.store.Save(newPref); err != nil {
		return nil, fmt.Errorf("failed to save new preference: %w", err)
	}

	return newPref, nil
}

// UpdateSections updates the monitored sections for a user.
func (pm *PreferenceManager) UpdateSections(chatID int64, sections []string) error {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return fmt.Errorf("failed to get preference: %w", err)
	}

	if pref == nil {
		return fmt.Errorf("preference not found for chat ID %d", chatID)
	}

	pref.Sections = sections

	if err := pm.store.Save(pref); err != nil {
		return fmt.Errorf("failed to save preference: %w", err)
	}

	return nil
}

// ToggleEnabled enables or disables notifications for a user.
func (pm *PreferenceManager) ToggleEnabled(chatID int64, enabled bool) error {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return fmt.Errorf("failed to get preference: %w", err)
	}

	if pref == nil {
		return fmt.Errorf("preference not found for chat ID %d", chatID)
	}

	pref.Enabled = enabled

	if err := pm.store.Save(pref); err != nil {
		return fmt.Errorf("failed to save preference: %w", err)
	}

	return nil
}

// List returns all stored user preferences.
func (pm *PreferenceManager) List() ([]UserPreference, error) {
	return pm.store.List()
}
