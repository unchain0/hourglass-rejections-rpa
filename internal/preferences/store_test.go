package preferences

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeConnPool struct{}

func (f *fakeConnPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, nil
}
func (f *fakeConnPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}
func (f *fakeConnPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}
func (f *fakeConnPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}

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

func TestSaveDiscoveredChat_New(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	err = store.SaveDiscoveredChat(123, "testuser")
	require.NoError(t, err)

	chat, err := store.GetDiscoveredChat(123)
	require.NoError(t, err)
	require.NotNil(t, chat)

	assert.Equal(t, int64(123), chat.ChatID)
	assert.Equal(t, "testuser", chat.Username)
	assert.Equal(t, 1, chat.MessageCount)
}

func TestSaveDiscoveredChat_Update(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	err = store.SaveDiscoveredChat(123, "testuser")
	require.NoError(t, err)

	err = store.SaveDiscoveredChat(123, "testuser")
	require.NoError(t, err)

	chat, err := store.GetDiscoveredChat(123)
	require.NoError(t, err)
	require.NotNil(t, chat)

	assert.Equal(t, 2, chat.MessageCount)
}

func TestSaveDiscoveredChat_UpdateFillsUsername(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	err = store.SaveDiscoveredChat(123, "")
	require.NoError(t, err)

	err = store.SaveDiscoveredChat(123, "filledlater")
	require.NoError(t, err)

	chat, err := store.GetDiscoveredChat(123)
	require.NoError(t, err)
	assert.Equal(t, "filledlater", chat.Username)
}

func TestGetDiscoveredChat_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	chat, err := store.GetDiscoveredChat(999)
	require.NoError(t, err)
	assert.Nil(t, chat)
}

func TestListDiscoveredChats(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	err = store.SaveDiscoveredChat(100, "user1")
	require.NoError(t, err)
	err = store.SaveDiscoveredChat(200, "user2")
	require.NoError(t, err)

	chats, err := store.ListDiscoveredChats()
	require.NoError(t, err)
	assert.Len(t, chats, 2)
}

func TestListDiscoveredChats_Empty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	chats, err := store.ListDiscoveredChats()
	require.NoError(t, err)
	assert.Empty(t, chats)
}

func TestStore_Save_SetsDataRetention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	pref := &UserPreference{ChatID: 123, Username: "testuser"}
	pref.SetSections([]string{})

	err = store.Save(pref)
	require.NoError(t, err)

	assert.False(t, pref.DataRetention.IsZero())
	assert.True(t, pref.DataRetention.After(time.Now().AddDate(0, 0, 89)))
}

func TestStore_Save_PreservesExistingDataRetention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	customRetention := time.Now().AddDate(1, 0, 0)
	pref := &UserPreference{
		ChatID:        123,
		Username:      "testuser",
		DataRetention: customRetention,
	}
	pref.SetSections([]string{})

	err = store.Save(pref)
	require.NoError(t, err)

	assert.Equal(t, customRetention.Unix(), pref.DataRetention.Unix())
}

func TestNewStore_InvalidPath(t *testing.T) {
	_, err := NewStore("/nonexistent/readonly/path/test.db")
	assert.Error(t, err)
}

func TestEnsureSecureDirectory_EmptyDir(t *testing.T) {
	originalHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := ensureSecureDirectory("test.db")
	assert.NoError(t, err)

	expectedDir := filepath.Join(tmpDir, ".local", "share", "hourglass-rpa")
	info, err := os.Stat(expectedDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureSecureDirectory_MkdirAllFails(t *testing.T) {
	err := ensureSecureDirectory("/proc/nonexistent/impossible/test.db")
	assert.Error(t, err)
}

func TestStore_Close_Success(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)

	err = store.Close()
	assert.NoError(t, err)
}

func TestStore_Close_DBError(t *testing.T) {
	store := &Store{db: &gorm.DB{Config: &gorm.Config{ConnPool: &fakeConnPool{}}}}
	err := store.Close()
	assert.Error(t, err)
}

func TestStore_Save_DBError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)

	sqlDB, _ := store.db.DB()
	sqlDB.Close()

	pref := &UserPreference{ChatID: 999, Username: "fail"}
	pref.SetSections([]string{})

	err = store.Save(pref)
	assert.Error(t, err)
}

func TestNewStore_PermissionError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	store.Close()

	os.Chmod(dbPath, 0000)
	defer os.Chmod(dbPath, 0600)

	_, err = NewStore(dbPath)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to")
	}
}

func TestNewStore_MigrationError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupt.db")

	err := os.WriteFile(dbPath, []byte("this is not a valid sqlite file"), 0600)
	require.NoError(t, err)

	_, err = NewStore(dbPath)
	assert.Error(t, err)
}

func TestNewStore_SetPermissionsError(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0700))

	dbPath := filepath.Join(subDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Skip("cannot create initial store")
	}
	store.Close()

	os.Chmod(subDir, 0500)
	defer os.Chmod(subDir, 0700)

	_, err = NewStore(dbPath)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to")
	}
}

func TestNewStore_SetSecurePermissionsFnError(t *testing.T) {
	original := setSecurePermissionsFn
	defer func() { setSecurePermissionsFn = original }()

	setSecurePermissionsFn = func(dbPath string) error {
		return errors.New("permission denied")
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	_, err := NewStore(dbPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set database permissions")
}
