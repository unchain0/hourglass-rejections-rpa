package preferences

import (
	"errors"
	"testing"
)

type mockStore struct {
	getFunc    func(chatID int64) (*UserPreference, error)
	saveFunc   func(pref *UserPreference) error
	deleteFunc func(chatID int64) error
	listFunc   func() ([]UserPreference, error)
}

func (m *mockStore) Get(chatID int64) (*UserPreference, error) {
	if m.getFunc != nil {
		return m.getFunc(chatID)
	}
	return nil, nil
}

func (m *mockStore) Save(pref *UserPreference) error {
	if m.saveFunc != nil {
		return m.saveFunc(pref)
	}
	return nil
}

func (m *mockStore) Delete(chatID int64) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(chatID)
	}
	return nil
}

func (m *mockStore) List() ([]UserPreference, error) {
	if m.listFunc != nil {
		return m.listFunc()
	}
	return nil, nil
}

func TestNewPreferenceManager(t *testing.T) {
	store := &mockStore{}
	pm := NewPreferenceManager(store)
	if pm == nil {
		t.Fatal("expected pm to not be nil")
	}
	if pm.store != store {
		t.Error("expected store to be set")
	}
}

func TestPreferenceManager_Get(t *testing.T) {
	expected := &UserPreference{ChatID: 123}
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			if chatID == 123 {
				return expected, nil
			}
			return nil, nil
		},
	}
	pm := NewPreferenceManager(store)

	pref, err := pm.Get(123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if pref != expected {
		t.Errorf("expected %v, got %v", expected, pref)
	}
}

func TestPreferenceManager_Get_Error(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, errors.New("get error")
		},
	}
	pm := NewPreferenceManager(store)

	_, err := pm.Get(123)
	if err == nil || err.Error() != "get error" {
		t.Errorf("expected get error, got %v", err)
	}
}

func TestPreferenceManager_GetOrCreate_Exists(t *testing.T) {
	expected := &UserPreference{ChatID: 123, Username: "existing"}
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return expected, nil
		},
	}
	pm := NewPreferenceManager(store)

	pref, err := pm.GetOrCreate(123, "test")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if pref != expected {
		t.Errorf("expected %v, got %v", expected, pref)
	}
}

func TestPreferenceManager_GetOrCreate_New(t *testing.T) {
	var saved *UserPreference
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, nil
		},
		saveFunc: func(pref *UserPreference) error {
			saved = pref
			return nil
		},
	}
	pm := NewPreferenceManager(store)

	pref, err := pm.GetOrCreate(123, "testuser")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if pref == nil {
		t.Fatal("expected pref to not be nil")
	}
	if pref.ChatID != 123 {
		t.Errorf("expected ChatID 123, got %d", pref.ChatID)
	}
	if pref.Username != "testuser" {
		t.Errorf("expected Username testuser, got %s", pref.Username)
	}
	if !pref.Enabled {
		t.Error("expected Enabled to be true")
	}
	if saved == nil {
		t.Error("expected Save to be called")
	}
}

func TestPreferenceManager_GetOrCreate_GetError(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, errors.New("get error")
		},
	}
	pm := NewPreferenceManager(store)

	_, err := pm.GetOrCreate(123, "test")
	if err == nil || err.Error() != "get error" {
		t.Errorf("expected get error, got %v", err)
	}
}

func TestPreferenceManager_GetOrCreate_SaveError(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, nil
		},
		saveFunc: func(pref *UserPreference) error {
			return errors.New("save error")
		},
	}
	pm := NewPreferenceManager(store)

	_, err := pm.GetOrCreate(123, "test")
	if err == nil || err.Error() != "save error" {
		t.Errorf("expected save error, got %v", err)
	}
}

func TestPreferenceManager_UpdateSections(t *testing.T) {
	pref := &UserPreference{ChatID: 123}
	pref.SetSections([]string{})

	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return pref, nil
		},
		saveFunc: func(p *UserPreference) error {
			return nil
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.UpdateSections(123, []string{"Campo", "Partes Mecânicas"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	sections := pref.Sections()
	if len(sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(sections))
	}
}

func TestPreferenceManager_UpdateSections_GetError(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, errors.New("get error")
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.UpdateSections(123, []string{"Campo"})
	if err == nil || err.Error() != "get error" {
		t.Errorf("expected get error, got %v", err)
	}
}

func TestPreferenceManager_UpdateSections_NotFound(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, nil
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.UpdateSections(123, []string{"Campo"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestPreferenceManager_UpdateSections_SaveError(t *testing.T) {
	pref := &UserPreference{ChatID: 123}
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return pref, nil
		},
		saveFunc: func(p *UserPreference) error {
			return errors.New("save error")
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.UpdateSections(123, []string{"Campo"})
	if err == nil || err.Error() != "save error" {
		t.Errorf("expected save error, got %v", err)
	}
}

func TestPreferenceManager_ToggleEnabled(t *testing.T) {
	pref := &UserPreference{ChatID: 123, Enabled: true}

	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return pref, nil
		},
		saveFunc: func(p *UserPreference) error {
			return nil
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.ToggleEnabled(123, false)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if pref.Enabled {
		t.Error("expected Enabled to be false")
	}
}

func TestPreferenceManager_ToggleEnabled_GetError(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, errors.New("get error")
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.ToggleEnabled(123, false)
	if err == nil || err.Error() != "get error" {
		t.Errorf("expected get error, got %v", err)
	}
}

func TestPreferenceManager_ToggleEnabled_NotFound(t *testing.T) {
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return nil, nil
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.ToggleEnabled(123, false)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestPreferenceManager_ToggleEnabled_SaveError(t *testing.T) {
	pref := &UserPreference{ChatID: 123, Enabled: true}
	store := &mockStore{
		getFunc: func(chatID int64) (*UserPreference, error) {
			return pref, nil
		},
		saveFunc: func(p *UserPreference) error {
			return errors.New("save error")
		},
	}
	pm := NewPreferenceManager(store)

	err := pm.ToggleEnabled(123, false)
	if err == nil || err.Error() != "save error" {
		t.Errorf("expected save error, got %v", err)
	}
}

func TestPreferenceManager_List(t *testing.T) {
	expected := []UserPreference{
		{ChatID: 123},
		{ChatID: 456},
	}
	store := &mockStore{
		listFunc: func() ([]UserPreference, error) {
			return expected, nil
		},
	}
	pm := NewPreferenceManager(store)

	prefs, err := pm.List()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(prefs) != 2 {
		t.Errorf("expected 2 prefs, got %d", len(prefs))
	}
}

func TestPreferenceManager_List_Error(t *testing.T) {
	store := &mockStore{
		listFunc: func() ([]UserPreference, error) {
			return nil, errors.New("list error")
		},
	}
	pm := NewPreferenceManager(store)

	_, err := pm.List()
	if err == nil || err.Error() != "list error" {
		t.Errorf("expected list error, got %v", err)
	}
}
