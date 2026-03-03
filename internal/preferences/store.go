package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PreferenceStore interface {
	Get(chatID int64) (*UserPreference, error)
	Save(pref *UserPreference) error
	Delete(chatID int64) error
	List() ([]UserPreference, error)
}

// UserPreference stores user notification preferences
// PII Classification: HIGH SENSITIVITY - Contains Telegram Chat IDs
type UserPreference struct {
	ID           uint   `gorm:"primaryKey"`
	ChatID       int64  `gorm:"uniqueIndex;not null"`
	Username     string `gorm:"size:255"`
	SectionsJSON string `gorm:"column:sections"`
	Enabled      bool   `gorm:"default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// GDPR compliance field - auto-delete after this date
	DataRetention time.Time `gorm:"index"`
}

// AuditLog tracks data access for GDPR compliance
type AuditLog struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index"`
	Action    string    `gorm:"size:100"` // data_accessed, data_modified, data_deleted
	Timestamp time.Time `gorm:"index"`
	Purpose   string    `gorm:"size:100"`
}

type JobExecution struct {
	ID         uint      `gorm:"primaryKey"`
	JobName    string    `gorm:"index;not null"`
	ExecutedAt time.Time `gorm:"index;not null"`
	Success    bool
	ErrorMsg   string
}

// DiscoveredChat stores chat IDs of users who messaged the bot
// This is separate from whitelist - admins can review and add to whitelist
type DiscoveredChat struct {
	ID           uint      `gorm:"primaryKey"`
	ChatID       int64     `gorm:"uniqueIndex;not null"`
	Username     string    `gorm:"size:255"`
	FirstSeen    time.Time `gorm:"index;not null"`
	LastSeen     time.Time `gorm:"index;not null"`
	MessageCount int       `gorm:"default:1"`
}

func (u *UserPreference) Sections() []string {
	var sections []string
	if err := json.Unmarshal([]byte(u.SectionsJSON), &sections); err != nil {
		return []string{}
	}
	return sections
}

func (u *UserPreference) SetSections(sections []string) {
	data, _ := json.Marshal(sections)
	u.SectionsJSON = string(data)
}

type Store struct {
	db *gorm.DB
}

var setSecurePermissionsFn = setSecurePermissions

// NewStore creates a SQLite database for user preferences
func NewStore(dbPath string) (*Store, error) {
	if err := ensureSecureDirectory(dbPath); err != nil {
		return nil, fmt.Errorf("failed to ensure secure directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.Exec("PRAGMA foreign_keys = ON")
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA synchronous = NORMAL")
	db.Exec("PRAGMA busy_timeout = 5000")

	if err := db.AutoMigrate(&UserPreference{}, &JobExecution{}, &AuditLog{}, &DiscoveredChat{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	if err := setSecurePermissionsFn(dbPath); err != nil {
		return nil, fmt.Errorf("failed to set database permissions: %w", err)
	}

	return &Store{db: db}, nil
}

func ensureSecureDirectory(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "" || dir == "." {
		dir = filepath.Join(os.Getenv("HOME"), ".local", "share", "hourglass-rpa")
	}

	// Create directory with restrictive permissions (owner only)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	return nil
}

func setSecurePermissions(dbPath string) error {
	// Skip for in-memory databases
	if dbPath == ":memory:" || strings.HasPrefix(dbPath, "file::memory:") {
		return nil
	}
	// Set file permissions to owner read/write only (0600)
	return os.Chmod(dbPath, 0600)
}

func (s *Store) Get(chatID int64) (*UserPreference, error) {
	var pref UserPreference
	result := s.db.Where("chat_id = ?", chatID).First(&pref)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}

	// Log access for audit trail
	s.logAccess(pref.ID, "data_accessed", "preference_retrieval")

	return &pref, result.Error
}

func (s *Store) Save(pref *UserPreference) error {
	// Set data retention period (GDPR: 90 days default)
	if pref.DataRetention.IsZero() {
		pref.DataRetention = time.Now().AddDate(0, 0, 90)
	}

	action := "data_modified"
	if pref.ID == 0 {
		action = "data_created"
	}

	if err := s.db.Save(pref).Error; err != nil {
		return err
	}

	s.logAccess(pref.ID, action, "preference_update")
	return nil
}

func (s *Store) Delete(chatID int64) error {
	var pref UserPreference
	if err := s.db.Where("chat_id = ?", chatID).First(&pref).Error; err != nil {
		return err
	}

	s.logAccess(pref.ID, "data_deleted", "user_deletion")

	return s.db.Where("chat_id = ?", chatID).Delete(&UserPreference{}).Error
}

func (s *Store) List() ([]UserPreference, error) {
	var prefs []UserPreference
	result := s.db.Find(&prefs)

	// Log bulk access
	for _, pref := range prefs {
		s.logAccess(pref.ID, "data_accessed", "bulk_listing")
	}

	return prefs, result.Error
}

func (s *Store) logAccess(userID uint, action, purpose string) {
	// Fire-and-forget audit logging
	s.db.Create(&AuditLog{
		UserID:    userID,
		Action:    action,
		Timestamp: time.Now(),
		Purpose:   purpose,
	})
}

func (s *Store) RecordJobExecution(jobName string, success bool, errorMsg string) error {
	return s.db.Create(&JobExecution{
		JobName:    jobName,
		ExecutedAt: time.Now(),
		Success:    success,
		ErrorMsg:   errorMsg,
	}).Error
}

func (s *Store) GetLastExecution(jobName string) (*time.Time, error) {
	var exec JobExecution
	result := s.db.Where("job_name = ?", jobName).Order("executed_at DESC").First(&exec)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &exec.ExecutedAt, result.Error
}

// CleanupExpiredData deletes user data past retention period (GDPR compliance)
func (s *Store) CleanupExpiredData() (int64, error) {
	result := s.db.Where("data_retention < ?", time.Now()).Delete(&UserPreference{})
	return result.RowsAffected, result.Error
}

// SaveDiscoveredChat saves or updates a discovered chat (user who messaged the bot)
func (s *Store) SaveDiscoveredChat(chatID int64, username string) error {
	var chat DiscoveredChat
	result := s.db.Where("chat_id = ?", chatID).First(&chat)

	now := time.Now()
	if result.Error == gorm.ErrRecordNotFound {
		// New chat - create record
		chat = DiscoveredChat{
			ChatID:       chatID,
			Username:     username,
			FirstSeen:    now,
			LastSeen:     now,
			MessageCount: 1,
		}
		return s.db.Create(&chat).Error
	}

	// Existing chat - update last seen and increment count
	chat.LastSeen = now
	chat.MessageCount++
	if username != "" && chat.Username == "" {
		chat.Username = username
	}
	return s.db.Save(&chat).Error
}

// GetDiscoveredChat retrieves a discovered chat by chatID
func (s *Store) GetDiscoveredChat(chatID int64) (*DiscoveredChat, error) {
	var chat DiscoveredChat
	result := s.db.Where("chat_id = ?", chatID).First(&chat)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &chat, result.Error
}

// ListDiscoveredChats returns all discovered chats
func (s *Store) ListDiscoveredChats() ([]DiscoveredChat, error) {
	var chats []DiscoveredChat
	result := s.db.Order("first_seen DESC").Find(&chats)
	return chats, result.Error
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
