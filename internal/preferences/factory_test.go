package preferences

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDatabaseConfigFromEnv_Defaults(t *testing.T) {
	os.Unsetenv("DB_TYPE")
	os.Unsetenv("DB_PATH")

	cfg := NewDatabaseConfigFromEnv()
	assert.Equal(t, "sqlite", cfg.Type)
	assert.Equal(t, "data/preferences.db", cfg.DSN)
}

func TestNewDatabaseConfigFromEnv_Postgres(t *testing.T) {
	os.Setenv("DB_TYPE", "postgres")
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
	os.Setenv("DB_SSLMODE", "disable")
	defer func() {
		os.Unsetenv("DB_TYPE")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("DB_SSLMODE")
	}()

	cfg := NewDatabaseConfigFromEnv()
	assert.Equal(t, "postgres", cfg.Type)
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, "5432", cfg.Port)
	assert.Equal(t, "testuser", cfg.User)
	assert.Equal(t, "testpass", cfg.Password)
	assert.Equal(t, "testdb", cfg.DBName)
	assert.Equal(t, "disable", cfg.SSLMode)
}

func TestDatabaseConfig_buildPostgresDSN(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "pass",
		DBName:   "mydb",
		SSLMode:  "disable",
	}

	dsn := cfg.buildPostgresDSN()
	assert.Contains(t, dsn, "host=localhost")
	assert.Contains(t, dsn, "port=5432")
	assert.Contains(t, dsn, "user=user")
	assert.Contains(t, dsn, "password=pass")
	assert.Contains(t, dsn, "dbname=mydb")
	assert.Contains(t, dsn, "sslmode=disable")
}

func TestDatabaseConfig_buildPostgresDSN_WithDSN(t *testing.T) {
	cfg := &DatabaseConfig{
		DSN: "postgres://user:pass@localhost/mydb?sslmode=disable",
	}

	dsn := cfg.buildPostgresDSN()
	assert.Equal(t, cfg.DSN, dsn)
}

func TestNewStoreFromConfig_InvalidType(t *testing.T) {
	cfg := &DatabaseConfig{
		Type: "invalid",
	}

	store, err := NewStoreFromConfig(cfg)
	assert.Error(t, err)
	assert.Nil(t, store)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestNewStoreFromConfig_NilConfig(t *testing.T) {
	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_PATH", ":memory:")
	defer func() {
		os.Unsetenv("DB_TYPE")
		os.Unsetenv("DB_PATH")
	}()

	store, err := NewStoreFromConfig(nil)
	require.NoError(t, err)
	assert.NotNil(t, store)
	store.Close()
}

func TestNewStoreFactory(t *testing.T) {
	cfg := &DatabaseConfig{Type: "sqlite", DSN: ":memory:"}
	factory := NewStoreFactory(cfg)
	assert.NotNil(t, factory)

	store, err := factory.CreateStore()
	require.NoError(t, err)
	assert.NotNil(t, store)
	store.Close()
}

func TestNewStoreWithPath(t *testing.T) {
	store, err := NewStoreWithPath(":memory:")
	require.NoError(t, err)
	assert.NotNil(t, store)
	store.Close()
}

func TestNewStoreFromConfig_SQLite_Error(t *testing.T) {
	cfg := &DatabaseConfig{
		Type: "sqlite",
		DSN:  "/invalid/path/that/cannot/be/created/test.db",
	}

	store, err := NewStoreFromConfig(cfg)
	assert.Error(t, err)
	assert.Nil(t, store)
}

func TestNewStoreWithPath_InvalidPath(t *testing.T) {
	store, err := NewStoreWithPath("/root/invalid/test.db")
	assert.Error(t, err)
	assert.Nil(t, store)
}

func TestNewStoreFromConfig_Postgres_Coverage(t *testing.T) {
	cfg := &DatabaseConfig{
		Type:     "postgres",
		Host:     "localhost",
		Port:     "5432",
		User:     "test",
		Password: "test",
		DBName:   "test",
		SSLMode:  "disable",
	}

	store, err := NewStoreFromConfig(cfg)
	assert.Error(t, err)
	assert.Nil(t, store)
}
