package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PreferenceStore defines the interface for preference persistence.
type PreferenceStore interface {
	Get(chatID int64) (*UserPreference, error)
	Save(pref *UserPreference) error
	Delete(chatID int64) error
	List() ([]UserPreference, error)
	ListBySection(section string) ([]UserPreference, error)
}

// FilePreferenceStore implements PreferenceStore using a JSON file.
type FilePreferenceStore struct {
	filePath string
	mutex    sync.RWMutex
}

// NewFilePreferenceStore creates a new file-backed preference store.
func NewFilePreferenceStore(filePath string) *FilePreferenceStore {
	return &FilePreferenceStore{
		filePath: filePath,
	}
}

// Get retrieves a user preference by chat ID.
func (s *FilePreferenceStore) Get(chatID int64) (*UserPreference, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	prefs, err := s.loadAll()
	if err != nil {
		return nil, err
	}

	for i := range prefs {
		if prefs[i].ChatID == chatID {
			return &prefs[i], nil
		}
	}

	return nil, nil
}

// Save persists a user preference, updating UpdatedAt timestamp.
func (s *FilePreferenceStore) Save(pref *UserPreference) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	prefs, err := s.loadAll()
	if err != nil {
		return err
	}

	pref.UpdatedAt = time.Now().UTC()

	found := false
	for i := range prefs {
		if prefs[i].ChatID == pref.ChatID {
			prefs[i] = *pref
			found = true

			break
		}
	}

	if !found {
		prefs = append(prefs, *pref)
	}

	return s.writeAll(prefs)
}

// Delete removes a user preference by chat ID.
func (s *FilePreferenceStore) Delete(chatID int64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	prefs, err := s.loadAll()
	if err != nil {
		return err
	}

	filtered := make([]UserPreference, 0, len(prefs))
	for _, p := range prefs {
		if p.ChatID != chatID {
			filtered = append(filtered, p)
		}
	}

	return s.writeAll(filtered)
}

// List returns all stored user preferences.
func (s *FilePreferenceStore) List() ([]UserPreference, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	prefs, err := s.loadAll()
	if err != nil {
		return nil, err
	}

	return prefs, nil
}

// ListBySection returns all enabled user preferences that include the given section.
func (s *FilePreferenceStore) ListBySection(section string) ([]UserPreference, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	prefs, err := s.loadAll()
	if err != nil {
		return nil, err
	}

	result := make([]UserPreference, 0)
	for _, p := range prefs {
		if !p.Enabled {
			continue
		}

		for _, sec := range p.Sections {
			if sec == section {
				result = append(result, p)

				break
			}
		}
	}

	return result, nil
}

// loadAll reads all preferences from the JSON file.
// Returns empty slice if file does not exist.
func (s *FilePreferenceStore) loadAll() ([]UserPreference, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []UserPreference{}, nil
		}

		return nil, fmt.Errorf("failed to read preferences file: %w", err)
	}

	if len(data) == 0 {
		return []UserPreference{}, nil
	}

	var prefs []UserPreference
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal preferences: %w", err)
	}

	if prefs == nil {
		prefs = []UserPreference{}
	}

	return prefs, nil
}

// writeAll persists all preferences to the JSON file with indentation.
func (s *FilePreferenceStore) writeAll(prefs []UserPreference) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create preferences directory: %w", err)
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write preferences file: %w", err)
	}

	return nil
}
