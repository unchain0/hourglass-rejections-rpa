package preferences

import (
	"time"
)

type PreferenceManager struct {
	store PreferenceStore
}

func NewPreferenceManager(store PreferenceStore) *PreferenceManager {
	return &PreferenceManager{store: store}
}

func (pm *PreferenceManager) Get(chatID int64) (*UserPreference, error) {
	return pm.store.Get(chatID)
}

func (pm *PreferenceManager) GetOrCreate(chatID int64, username string) (*UserPreference, error) {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return nil, err
	}
	if pref != nil {
		return pref, nil
	}

	now := time.Now().UTC()
	newPref := &UserPreference{
		ChatID:    chatID,
		Username:  username,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	newPref.SetSections([]string{})

	if err := pm.store.Save(newPref); err != nil {
		return nil, err
	}
	return newPref, nil
}

func (pm *PreferenceManager) UpdateSections(chatID int64, sections []string) error {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return err
	}
	if pref == nil {
		return nil
	}

	pref.SetSections(sections)
	pref.UpdatedAt = time.Now().UTC()
	return pm.store.Save(pref)
}

func (pm *PreferenceManager) ToggleEnabled(chatID int64, enabled bool) error {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return err
	}
	if pref == nil {
		return nil
	}

	pref.Enabled = enabled
	pref.UpdatedAt = time.Now().UTC()
	return pm.store.Save(pref)
}

func (pm *PreferenceManager) List() ([]UserPreference, error) {
	return pm.store.List()
}

// SetAuthorized grants or revokes bot access for a chat.
func (pm *PreferenceManager) SetAuthorized(chatID int64, username string, authorized bool) error {
	pref, err := pm.GetOrCreate(chatID, username)
	if err != nil {
		return err
	}
	pref.Authorized = authorized
	pref.UpdatedAt = time.Now().UTC()
	return pm.store.Save(pref)
}

// IsAuthorized reports whether a persisted user may access the bot.
func (pm *PreferenceManager) IsAuthorized(chatID int64) (bool, error) {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return false, err
	}
	return pref != nil && pref.Authorized, nil
}

// RecordDiscoveredChat saves a discovered chat (user who messaged the bot)
// This is separate from whitelist authorization - it just tracks who contacted the bot
func (pm *PreferenceManager) RecordDiscoveredChat(chatID int64, username string) error {
	// Try to cast to *Store to access the method
	if store, ok := pm.store.(*Store); ok {
		return store.SaveDiscoveredChat(chatID, username)
	}
	return nil
}

// GetLanguage returns the user's language preference, defaulting to "en"
func (pm *PreferenceManager) GetLanguage(chatID int64) string {
	pref, err := pm.store.Get(chatID)
	if err != nil || pref == nil {
		return "en"
	}
	if pref.Language == "" {
		return "en"
	}
	return pref.Language
}

// UpdateLanguage updates the user's language preference
func (pm *PreferenceManager) UpdateLanguage(chatID int64, language string) error {
	pref, err := pm.store.Get(chatID)
	if err != nil {
		return err
	}
	if pref == nil {
		return nil
	}

	pref.Language = language
	pref.UpdatedAt = time.Now().UTC()
	return pm.store.Save(pref)
}
