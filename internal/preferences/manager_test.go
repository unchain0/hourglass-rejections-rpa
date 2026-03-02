package preferences

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPreferenceManager(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)
	assert.NotNil(t, pm)
}

func TestPreferenceManager_GetOrCreate_New(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)

	pref, err := pm.GetOrCreate(123, "alice")
	require.NoError(t, err)
	require.NotNil(t, pref)
	assert.Equal(t, int64(123), pref.ChatID)
	assert.Equal(t, "alice", pref.Username)
	assert.NotNil(t, pref.Sections)
	assert.Empty(t, pref.Sections)
	assert.True(t, pref.Enabled)
	assert.False(t, pref.CreatedAt.IsZero())
	assert.False(t, pref.UpdatedAt.IsZero())
}

func TestPreferenceManager_GetOrCreate_Existing(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)

	first, err := pm.GetOrCreate(123, "alice")
	require.NoError(t, err)

	second, err := pm.GetOrCreate(123, "alice_changed")
	require.NoError(t, err)

	// Should return existing, not create new
	assert.Equal(t, first.ChatID, second.ChatID)
	assert.Equal(t, "alice", second.Username) // original username preserved
}

func TestPreferenceManager_UpdateSections(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)

	_, err := pm.GetOrCreate(123, "alice")
	require.NoError(t, err)

	sections := []string{"Campo", "Partes Mecânicas", "Testemunho Público"}
	err = pm.UpdateSections(123, sections)
	assert.NoError(t, err)

	pref, err := store.Get(123)
	require.NoError(t, err)
	assert.Equal(t, sections, pref.Sections)
}

func TestPreferenceManager_UpdateSections_NotFound(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)

	err := pm.UpdateSections(999, []string{"Campo"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "preference not found")
}

func TestPreferenceManager_ToggleEnabled(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)

	_, err := pm.GetOrCreate(123, "alice")
	require.NoError(t, err)

	// Disable
	err = pm.ToggleEnabled(123, false)
	assert.NoError(t, err)

	pref, err := store.Get(123)
	require.NoError(t, err)
	assert.False(t, pref.Enabled)

	// Re-enable
	err = pm.ToggleEnabled(123, true)
	assert.NoError(t, err)

	pref, err = store.Get(123)
	require.NoError(t, err)
	assert.True(t, pref.Enabled)
}

func TestPreferenceManager_ToggleEnabled_NotFound(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
	pm := NewPreferenceManager(store)

	err := pm.ToggleEnabled(999, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "preference not found")
}

// mockErrorStore is a mock that returns errors for testing error paths.
type mockErrorStore struct{}

func (m *mockErrorStore) Get(_ int64) (*UserPreference, error) {
	return nil, fmt.Errorf("store error")
}

func (m *mockErrorStore) Save(_ *UserPreference) error {
	return fmt.Errorf("store error")
}

func (m *mockErrorStore) Delete(_ int64) error {
	return fmt.Errorf("store error")
}

func (m *mockErrorStore) List() ([]UserPreference, error) {
	return nil, fmt.Errorf("store error")
}

func (m *mockErrorStore) ListBySection(_ string) ([]UserPreference, error) {
	return nil, fmt.Errorf("store error")
}

func TestPreferenceManager_GetOrCreate_GetError(t *testing.T) {
	pm := NewPreferenceManager(&mockErrorStore{})

	_, err := pm.GetOrCreate(123, "alice")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get preference")
}

func TestPreferenceManager_GetOrCreate_SaveError(t *testing.T) {
	pm := NewPreferenceManager(&mockSaveErrorStore{})

	_, err := pm.GetOrCreate(123, "alice")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save new preference")
}

func TestPreferenceManager_UpdateSections_GetError(t *testing.T) {
	pm := NewPreferenceManager(&mockErrorStore{})

	err := pm.UpdateSections(123, []string{"Campo"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get preference")
}

func TestPreferenceManager_UpdateSections_SaveError(t *testing.T) {
	pm := NewPreferenceManager(&mockSaveErrorOnUpdateStore{})

	err := pm.UpdateSections(123, []string{"Campo"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save preference")
}

func TestPreferenceManager_ToggleEnabled_GetError(t *testing.T) {
	pm := NewPreferenceManager(&mockErrorStore{})

	err := pm.ToggleEnabled(123, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get preference")
}

func TestPreferenceManager_ToggleEnabled_SaveError(t *testing.T) {
	pm := NewPreferenceManager(&mockSaveErrorOnUpdateStore{})

	err := pm.ToggleEnabled(123, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save preference")
}

// mockSaveErrorStore returns nil on Get (no existing pref) but error on Save.
type mockSaveErrorStore struct{}

func (m *mockSaveErrorStore) Get(_ int64) (*UserPreference, error) {
	return nil, nil
}

func (m *mockSaveErrorStore) Save(_ *UserPreference) error {
	return fmt.Errorf("save error")
}

func (m *mockSaveErrorStore) Delete(_ int64) error {
	return nil
}

func (m *mockSaveErrorStore) List() ([]UserPreference, error) {
	return []UserPreference{}, nil
}

func (m *mockSaveErrorStore) ListBySection(_ string) ([]UserPreference, error) {
	return []UserPreference{}, nil
}

// mockSaveErrorOnUpdateStore returns an existing pref on Get but error on Save.
type mockSaveErrorOnUpdateStore struct{}

func (m *mockSaveErrorOnUpdateStore) Get(_ int64) (*UserPreference, error) {
	return &UserPreference{
		ChatID:   123,
		Username: "test",
		Sections: []string{},
		Enabled:  true,
	}, nil
}

func (m *mockSaveErrorOnUpdateStore) Save(_ *UserPreference) error {
	return fmt.Errorf("save error")
}

func (m *mockSaveErrorOnUpdateStore) Delete(_ int64) error {
	return nil
}

func (m *mockSaveErrorOnUpdateStore) List() ([]UserPreference, error) {
	return []UserPreference{}, nil
}

func (m *mockSaveErrorOnUpdateStore) ListBySection(_ string) ([]UserPreference, error) {
	return []UserPreference{}, nil
}
