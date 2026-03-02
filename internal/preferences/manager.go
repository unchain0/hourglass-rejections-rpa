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
