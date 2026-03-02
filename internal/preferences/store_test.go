package preferences

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	assert.NotNil(t, store)
}

func TestStore_Get_NonExistent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	pref, err := store.Get(123)
	assert.NoError(t, err)
	assert.Nil(t, pref)
}

func TestStore_SaveAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	pref := &UserPreference{
		ChatID:   123,
		Username: "testuser",
		Enabled:  true,
	}
	pref.SetSections([]string{"Campo", "Partes Mecânicas"})

	err = store.Save(pref)
	require.NoError(t, err)

	retrieved, err := store.Get(123)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, int64(123), retrieved.ChatID)
	assert.Equal(t, "testuser", retrieved.Username)
	assert.True(t, retrieved.Enabled)
	assert.Equal(t, []string{"Campo", "Partes Mecânicas"}, retrieved.Sections())
}

func TestStore_Update(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	pref := &UserPreference{ChatID: 123, Username: "testuser"}
	pref.SetSections([]string{"Campo"})
	require.NoError(t, store.Save(pref))

	pref.Username = "updated"
	pref.SetSections([]string{"Campo", "Testemunho"})
	require.NoError(t, store.Save(pref))

	retrieved, err := store.Get(123)
	require.NoError(t, err)
	assert.Equal(t, "updated", retrieved.Username)
	assert.Equal(t, []string{"Campo", "Testemunho"}, retrieved.Sections())
}

func TestStore_Delete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	pref := &UserPreference{ChatID: 123}
	pref.SetSections([]string{})
	require.NoError(t, store.Save(pref))

	err = store.Delete(123)
	require.NoError(t, err)

	retrieved, err := store.Get(123)
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestStore_List(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	for i := 1; i <= 3; i++ {
		pref := &UserPreference{ChatID: int64(i), Username: "user"}
		pref.SetSections([]string{})
		require.NoError(t, store.Save(pref))
	}

	list, err := store.List()
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestJobExecution(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	err = store.RecordJobExecution("test-job", true, "")
	require.NoError(t, err)

	lastExec, err := store.GetLastExecution("test-job")
	require.NoError(t, err)
	require.NotNil(t, lastExec)

	err = store.RecordJobExecution("test-job", false, "error message")
	require.NoError(t, err)
}

func TestCleanupExpiredData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	pref := &UserPreference{ChatID: 123, Username: "testuser"}
	pref.SetSections([]string{"Campo"})
	require.NoError(t, store.Save(pref))

	count, err := store.CleanupExpiredData()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestGetLastExecution_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	lastExec, err := store.GetLastExecution("non-existent-job")
	require.NoError(t, err)
	assert.Nil(t, lastExec)
}

func TestSections_InvalidJSON(t *testing.T) {
	pref := &UserPreference{SectionsJSON: "invalid json"}
	sections := pref.Sections()
	assert.Equal(t, []string{}, sections)
}

func TestSetSections_MarshalError(t *testing.T) {
	pref := &UserPreference{}
	pref.SetSections(nil)
	assert.Equal(t, "null", pref.SectionsJSON)
}

func TestNewStore_InMemory(t *testing.T) {
	store, err := NewStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	assert.NotNil(t, store)
}

func TestDelete_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	err = store.Delete(999)
	assert.Error(t, err)
}
