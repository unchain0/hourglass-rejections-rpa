package preferences

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFilePreferenceStore(t *testing.T) {
	store := NewFilePreferenceStore("/tmp/test.json")
	assert.NotNil(t, store)
	assert.Equal(t, "/tmp/test.json", store.filePath)
}

func TestFilePreferenceStore_Get_NonExistentFile(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	pref, err := store.Get(123)
	assert.NoError(t, err)
	assert.Nil(t, pref)
}

func TestFilePreferenceStore_Get_Found(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	now := time.Now().UTC().Truncate(time.Second)
	err := store.Save(&UserPreference{
		ChatID:    123,
		Username:  "testuser",
		Sections:  []string{"Campo"},
		Enabled:   true,
		CreatedAt: now,
	})
	require.NoError(t, err)

	pref, err := store.Get(123)
	assert.NoError(t, err)
	require.NotNil(t, pref)
	assert.Equal(t, int64(123), pref.ChatID)
	assert.Equal(t, "testuser", pref.Username)
	assert.Equal(t, []string{"Campo"}, pref.Sections)
	assert.True(t, pref.Enabled)
}

func TestFilePreferenceStore_Get_NotFound(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	err := store.Save(&UserPreference{
		ChatID:   123,
		Username: "testuser",
		Sections: []string{},
		Enabled:  true,
	})
	require.NoError(t, err)

	pref, err := store.Get(999)
	assert.NoError(t, err)
	assert.Nil(t, pref)
}

func TestFilePreferenceStore_Save_NewPreference(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	pref := &UserPreference{
		ChatID:    100,
		Username:  "alice",
		Sections:  []string{"Partes Mecânicas"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
	}

	err := store.Save(pref)
	assert.NoError(t, err)
	assert.False(t, pref.UpdatedAt.IsZero())

	// Verify persisted
	loaded, err := store.Get(100)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "alice", loaded.Username)
}

func TestFilePreferenceStore_Save_UpdateExisting(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	pref := &UserPreference{
		ChatID:    100,
		Username:  "alice",
		Sections:  []string{"Campo"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.Save(pref))

	firstUpdate := pref.UpdatedAt

	// Update
	pref.Sections = []string{"Campo", "Partes Mecânicas"}
	time.Sleep(10 * time.Millisecond) // ensure time difference
	require.NoError(t, store.Save(pref))

	loaded, err := store.Get(100)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, []string{"Campo", "Partes Mecânicas"}, loaded.Sections)
	assert.True(t, loaded.UpdatedAt.After(firstUpdate) || loaded.UpdatedAt.Equal(firstUpdate))

	// Ensure only one entry
	all, err := store.List()
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestFilePreferenceStore_Save_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	store := NewFilePreferenceStore(filepath.Join(dir, "prefs.json"))

	err := store.Save(&UserPreference{
		ChatID:   1,
		Username: "test",
		Sections: []string{},
		Enabled:  true,
	})
	assert.NoError(t, err)

	// Verify directory was created
	_, err = os.Stat(dir)
	assert.NoError(t, err)
}

func TestFilePreferenceStore_Delete(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	require.NoError(t, store.Save(&UserPreference{ChatID: 1, Username: "a", Sections: []string{}}))
	require.NoError(t, store.Save(&UserPreference{ChatID: 2, Username: "b", Sections: []string{}}))

	err := store.Delete(1)
	assert.NoError(t, err)

	all, err := store.List()
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, int64(2), all[0].ChatID)
}

func TestFilePreferenceStore_Delete_NonExistent(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	require.NoError(t, store.Save(&UserPreference{ChatID: 1, Username: "a", Sections: []string{}}))

	err := store.Delete(999)
	assert.NoError(t, err)

	all, err := store.List()
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestFilePreferenceStore_Delete_EmptyFile(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	err := store.Delete(1)
	assert.NoError(t, err)
}

func TestFilePreferenceStore_List_Empty(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	prefs, err := store.List()
	assert.NoError(t, err)
	assert.NotNil(t, prefs)
	assert.Empty(t, prefs)
}

func TestFilePreferenceStore_List_Multiple(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	require.NoError(t, store.Save(&UserPreference{ChatID: 1, Username: "a", Sections: []string{}}))
	require.NoError(t, store.Save(&UserPreference{ChatID: 2, Username: "b", Sections: []string{}}))
	require.NoError(t, store.Save(&UserPreference{ChatID: 3, Username: "c", Sections: []string{}}))

	prefs, err := store.List()
	assert.NoError(t, err)
	assert.Len(t, prefs, 3)
}

func TestFilePreferenceStore_ListBySection(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	require.NoError(t, store.Save(&UserPreference{
		ChatID: 1, Username: "a", Sections: []string{"Campo", "Partes Mecânicas"}, Enabled: true,
	}))
	require.NoError(t, store.Save(&UserPreference{
		ChatID: 2, Username: "b", Sections: []string{"Campo"}, Enabled: true,
	}))
	require.NoError(t, store.Save(&UserPreference{
		ChatID: 3, Username: "c", Sections: []string{"Partes Mecânicas"}, Enabled: true,
	}))
	require.NoError(t, store.Save(&UserPreference{
		ChatID: 4, Username: "d", Sections: []string{"Campo"}, Enabled: false, // disabled
	}))

	result, err := store.ListBySection("Campo")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2) // users 1 and 2 (user 4 is disabled)

	result, err = store.ListBySection("Partes Mecânicas")
	assert.NoError(t, err)
	assert.Len(t, result, 2) // users 1 and 3
}

func TestFilePreferenceStore_ListBySection_Empty(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	result, err := store.ListBySection("NonExistent")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFilePreferenceStore_ListBySection_NoMatch(t *testing.T) {
	store := NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))

	require.NoError(t, store.Save(&UserPreference{
		ChatID: 1, Username: "a", Sections: []string{"Campo"}, Enabled: true,
	}))

	result, err := store.ListBySection("NonExistent")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFilePreferenceStore_LoadAll_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(fp, []byte(""), 0600))

	store := NewFilePreferenceStore(fp)
	prefs, err := store.List()
	assert.NoError(t, err)
	assert.NotNil(t, prefs)
	assert.Empty(t, prefs)
}

func TestFilePreferenceStore_LoadAll_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(fp, []byte("not json"), 0600))

	store := NewFilePreferenceStore(fp)
	_, err := store.List()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal preferences")
}

func TestFilePreferenceStore_LoadAll_ReadError(t *testing.T) {
	// Use a directory path as file path to trigger read error
	dir := t.TempDir()
	store := NewFilePreferenceStore(dir) // dir is not a file

	_, err := store.List()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read preferences file")
}

func TestFilePreferenceStore_WriteAll_DirectoryError(t *testing.T) {
	// Use a valid temp dir but make the target nested under /proc to fail mkdir
	store := NewFilePreferenceStore("/proc/1/invalid/nested/prefs.json")

	err := store.Save(&UserPreference{ChatID: 1, Username: "a", Sections: []string{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create preferences directory")
}

func TestFilePreferenceStore_WriteAll_WriteError(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "prefs.json")
	// Write a valid empty JSON array so loadAll succeeds
	require.NoError(t, os.WriteFile(fp, []byte("[]"), 0600))

	// Make file read-only so WriteFile fails
	require.NoError(t, os.Chmod(fp, 0400))
	t.Cleanup(func() {
		_ = os.Chmod(fp, 0600) // restore for cleanup
	})

	store := NewFilePreferenceStore(fp)

	err := store.Save(&UserPreference{ChatID: 1, Username: "a", Sections: []string{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write preferences file")
}

func TestFilePreferenceStore_Get_ReadError(t *testing.T) {
	dir := t.TempDir()
	store := NewFilePreferenceStore(dir)

	_, err := store.Get(1)
	assert.Error(t, err)
}

func TestFilePreferenceStore_Delete_ReadError(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(fp, []byte("bad json"), 0600))

	store := NewFilePreferenceStore(fp)
	err := store.Delete(1)
	assert.Error(t, err)
}

func TestFilePreferenceStore_Save_ReadError(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(fp, []byte("bad json"), 0600))

	store := NewFilePreferenceStore(fp)
	err := store.Save(&UserPreference{ChatID: 1, Username: "a", Sections: []string{}})
	assert.Error(t, err)
}

func TestFilePreferenceStore_ListBySection_ReadError(t *testing.T) {
	dir := t.TempDir()
	store := NewFilePreferenceStore(dir)

	_, err := store.ListBySection("Campo")
	assert.Error(t, err)
}

func TestFilePreferenceStore_LoadAll_NullJSON(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(fp, []byte("null"), 0600))

	store := NewFilePreferenceStore(fp)
	prefs, err := store.List()
	assert.NoError(t, err)
	assert.NotNil(t, prefs)
	assert.Empty(t, prefs)
}
