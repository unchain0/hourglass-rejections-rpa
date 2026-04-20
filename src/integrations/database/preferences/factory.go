package preferences

import (
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DatabaseConfig struct {
	Type     string
	DSN      string
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func NewDatabaseConfigFromEnv() *DatabaseConfig {
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	if dbType == "postgres" {
		return &DatabaseConfig{
			Type:     dbType,
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", ""),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "hourglass"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		}
	}

	return &DatabaseConfig{
		Type: dbType,
		DSN:  getEnv("DB_PATH", "data/preferences.db"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (c *DatabaseConfig) buildPostgresDSN() string {
	if c.DSN != "" {
		return c.DSN
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

func NewStoreFromConfig(cfg *DatabaseConfig) (*Store, error) {
	if cfg == nil {
		cfg = NewDatabaseConfigFromEnv()
	}

	switch cfg.Type {
	case "postgres":
		return newPostgresStore(cfg)
	case "sqlite":
		return newSQLiteStore(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

var autoMigrateFn = func(db *gorm.DB) error {
	return db.AutoMigrate(&UserPreference{}, &JobExecution{}, &AuditLog{}, &DiscoveredChat{}, &RejectionLog{})
}

var openSQLiteDBFn = func(dbPath string) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

var openPostgresDBFn = func(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

var execPragmaFn = func(db *gorm.DB, query string) error {
	return db.Exec(query).Error
}

func newSQLiteStore(dbPath string) (*Store, error) {
	if err := ensureSecureDirectory(dbPath); err != nil {
		return nil, fmt.Errorf("failed to ensure secure directory: %w", err)
	}

	db, err := openSQLiteDBFn(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := execPragmaFn(db, "PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to set PRAGMA foreign_keys: %w", err)
	}
	if err := execPragmaFn(db, "PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("failed to set PRAGMA journal_mode: %w", err)
	}
	if err := execPragmaFn(db, "PRAGMA synchronous = NORMAL"); err != nil {
		return nil, fmt.Errorf("failed to set PRAGMA synchronous: %w", err)
	}
	if err := execPragmaFn(db, "PRAGMA busy_timeout = 5000"); err != nil {
		return nil, fmt.Errorf("failed to set PRAGMA busy_timeout: %w", err)
	}

	if err := autoMigrateFn(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	if err := setSecurePermissionsFn(dbPath); err != nil {
		return nil, fmt.Errorf("failed to set database permissions: %w", err)
	}

	return &Store{db: db}, nil
}

func newPostgresStore(cfg *DatabaseConfig) (*Store, error) {
	dsn := cfg.buildPostgresDSN()

	db, err := openPostgresDBFn(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	if err := autoMigrateFn(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return &Store{db: db}, nil
}

type StoreFactory struct {
	config *DatabaseConfig
}

func NewStoreFactory(cfg *DatabaseConfig) *StoreFactory {
	return &StoreFactory{config: cfg}
}

func (f *StoreFactory) CreateStore() (*Store, error) {
	return NewStoreFromConfig(f.config)
}

func NewStoreWithPath(dbPath string) (*Store, error) {
	return newSQLiteStore(dbPath)
}
