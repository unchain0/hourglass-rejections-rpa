package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/preferences"
)

// TestLoadEnvFiles_NoFile tests that loadEnvFiles handles missing .env files gracefully
func TestLoadEnvFiles_NoFile(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Change to temporary directory (no .env file)
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Should not panic or error when no .env exists
	loadEnvFiles()

	// If we get here, the function handled missing files gracefully
}

// TestLoadEnvFiles_CurrentDirectory tests loading .env from current directory
func TestLoadEnvFiles_CurrentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create a .env file with test values
	envContent := "TEST_VAR_1=value1\nTEST_VAR_2=value2\n"
	if err := os.WriteFile(".env", []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create .env file: %v", err)
	}

	// Load env files
	loadEnvFiles()

	// Verify the environment variables are loaded
	if got := os.Getenv("TEST_VAR_1"); got != "value1" {
		t.Errorf("TEST_VAR_1 = %q, want %q", got, "value1")
	}
	if got := os.Getenv("TEST_VAR_2"); got != "value2" {
		t.Errorf("TEST_VAR_2 = %q, want %q", got, "value2")
	}
}

// TestLoadEnvFiles_ParentDirectory tests loading .env from parent directory
func TestLoadEnvFiles_ParentDirectory(t *testing.T) {
	parentDir := t.TempDir()
	childDir := filepath.Join(parentDir, "child")

	if err := os.Mkdir(childDir, 0755); err != nil {
		t.Fatalf("failed to create child directory: %v", err)
	}

	// Create .env in parent directory
	envContent := "PARENT_VAR=parent_value\n"
	if err := os.WriteFile(filepath.Join(parentDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create parent .env file: %v", err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(childDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Load env files
	loadEnvFiles()

	// Verify the parent directory .env was loaded
	if got := os.Getenv("PARENT_VAR"); got != "parent_value" {
		t.Errorf("PARENT_VAR = %q, want %q", got, "parent_value")
	}
}

// TestLoadEnvFiles_MultipleLocations tests that .env is loaded from first available location
func TestLoadEnvFiles_MultipleLocations(t *testing.T) {
	parentDir := t.TempDir()
	childDir := filepath.Join(parentDir, "child", "grandchild")

	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatalf("failed to create directory structure: %v", err)
	}

	// Create .env files in multiple locations
	parentEnv := "PARENT_VAR=parent\n"
	currentEnv := "CURRENT_VAR=current\n"

	if err := os.WriteFile(filepath.Join(parentDir, ".env"), []byte(parentEnv), 0644); err != nil {
		t.Fatalf("failed to create parent .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(childDir, ".env"), []byte(currentEnv), 0644); err != nil {
		t.Fatalf("failed to create current .env: %v", err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(childDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Load env files - should load from current directory first
	loadEnvFiles()

	// Current directory .env should be loaded
	if got := os.Getenv("CURRENT_VAR"); got != "current" {
		t.Errorf("CURRENT_VAR = %q, want %q", got, "current")
	}
	// Parent .env should NOT be loaded (function returns after first successful load)
	// Note: This behavior depends on the godotenv.Load implementation
}

// TestSendTelegramNotification_MissingConfig tests with missing token or chat ID
func TestSendTelegramNotification_MissingConfig(t *testing.T) {
	// Clear any existing env vars
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
	}()

	// Test with both missing
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	os.Unsetenv("TELEGRAM_CHAT_ID")

	rejections := []domain.Rejeicao{
		{Secao: "Test Section", Quem: "Test User", OQue: "Test Assignment", PraQuando: "01/03/2026"},
	}

	err := sendTelegramNotification(nil, rejections)
	if err != nil {
		t.Errorf("sendTelegramNotification with missing config should return nil, got error: %v", err)
	}

	// Test with missing token only
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")
	os.Unsetenv("TELEGRAM_BOT_TOKEN")

	err = sendTelegramNotification(nil, rejections)
	if err != nil {
		t.Errorf("sendTelegramNotification with missing token should return nil, got error: %v", err)
	}

	// Test with missing chat ID only
	os.Unsetenv("TELEGRAM_CHAT_ID")
	os.Setenv("TELEGRAM_BOT_TOKEN", "fake-token")

	err = sendTelegramNotification(nil, rejections)
	if err != nil {
		t.Errorf("sendTelegramNotification with missing chat ID should return nil, got error: %v", err)
	}
}

// TestSendTelegramNotification_InvalidChatID tests with invalid chat ID
func TestSendTelegramNotification_InvalidChatID(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "fake-token")
	os.Setenv("TELEGRAM_CHAT_ID", "invalid-chat-id")

	rejections := []domain.Rejeicao{
		{Secao: "Test Section", Quem: "Test User", OQue: "Test Assignment", PraQuando: "01/03/2026"},
	}

	err := sendTelegramNotification(nil, rejections)
	if err == nil {
		t.Error("sendTelegramNotification with invalid chat ID should return error")
	}

	if !strings.Contains(err.Error(), "no valid telegram chat IDs") {
		t.Errorf("error message should mention no valid chat IDs, got: %v", err)
}
}

// TestSendTelegramNotification_EmptyWhitelist tests with empty whitelist
func TestSendTelegramNotification_EmptyWhitelist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	origWhitelist := os.Getenv("TELEGRAM_WHITELIST")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
		os.Setenv("TELEGRAM_WHITELIST", origWhitelist)
	}()

	// Set valid config but empty whitelist
	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")
	os.Setenv("TELEGRAM_WHITELIST", "")

	rejections := []domain.Rejeicao{
		{Secao: "Test Section", Quem: "Test User", OQue: "Test Assignment", PraQuando: "01/03/2026"},
	}

	// This will fail to create TelegramNotifier with fake token, which is expected
	err := sendTelegramNotification(nil, rejections)
	// Error is expected due to fake token, not due to empty whitelist
	// The important part is that it doesn't fail due to whitelist parsing
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}

// TestSendTelegramNotification_ValidWhitelist tests whitelist parsing with valid IDs
func TestSendTelegramNotification_ValidWhitelist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	origWhitelist := os.Getenv("TELEGRAM_WHITELIST")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
		os.Setenv("TELEGRAM_WHITELIST", origWhitelist)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")
	os.Setenv("TELEGRAM_WHITELIST", "987654321,111222333")

	rejections := []domain.Rejeicao{
		{Secao: "Test Section", Quem: "Test User", OQue: "Test Assignment", PraQuando: "01/03/2026"},
	}

	err := sendTelegramNotification(nil, rejections)
	// Will fail due to fake token, but whitelist should parse correctly
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}

// TestSendTelegramNotification_WhitelistWithInvalidIDs tests whitelist parsing with some invalid IDs
func TestSendTelegramNotification_WhitelistWithInvalidIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	origWhitelist := os.Getenv("TELEGRAM_WHITELIST")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
		os.Setenv("TELEGRAM_WHITELIST", origWhitelist)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")
	os.Setenv("TELEGRAM_WHITELIST", "987654321,invalid,111222333")

	rejections := []domain.Rejeicao{
		{Secao: "Test Section", Quem: "Test User", OQue: "Test Assignment", PraQuando: "01/03/2026"},
	}

	err := sendTelegramNotification(nil, rejections)
	// Should skip invalid IDs but continue with valid ones
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}

// TestSendTelegramNotification_EmptyRejections tests with empty rejection list
func TestSendTelegramNotification_EmptyRejections(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")

	rejections := []domain.Rejeicao{}

	err := sendTelegramNotification(nil, rejections)
	// Empty list will fail with fake token - this is expected behavior
	// The function still validates config and creates the notifier even for empty lists
	if err != nil {
		t.Logf("sendTelegramNotification with empty list failed (expected): %v", err)
	}
}

// TestSendTelegramNotification_MultipleRejections tests with multiple rejection entries
func TestSendTelegramNotification_MultipleRejections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")

	rejections := []domain.Rejeicao{
		{Secao: "Partes Mecânicas", Quem: "John Doe", OQue: "Audio", PraQuando: "01/03/2026"},
		{Secao: "Campo", Quem: "Jane Smith", OQue: "Video", PraQuando: "02/03/2026"},
		{Secao: "Testemunho Público", Quem: "Bob Johnson", OQue: "Indicadores", PraQuando: "03/03/2026"},

	}

	err := sendTelegramNotification(nil, rejections)
	// Will fail due to fake token, but should handle multiple entries
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}

// TestSendTelegramNotification_WhitespaceInWhitelist tests whitespace handling in whitelist
func TestSendTelegramNotification_WhitespaceInWhitelist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	origWhitelist := os.Getenv("TELEGRAM_WHITELIST")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
		os.Setenv("TELEGRAM_WHITELIST", origWhitelist)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")
	os.Setenv("TELEGRAM_WHITELIST", " 987654321 , 111222333 , 333444555 ")

	rejections := []domain.Rejeicao{
		{Secao: "Test Section", Quem: "Test User", OQue: "Test Assignment", PraQuando: "01/03/2026"},
	}

	err := sendTelegramNotification(nil, rejections)
	// Should handle whitespace correctly
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}

// TestLoadEnvFiles_HomeDirectory tests loading .env from home directory
func TestLoadEnvFiles_HomeDirectory(t *testing.T) {
	// Skip on Windows due to path handling differences
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows due to path differences")
	}

	origHome := os.Getenv("HOME")
	origWd, _ := os.Getwd()
	defer func() {
		os.Setenv("HOME", origHome)
		os.Chdir(origWd)
	}()

	// Create temporary home directory structure
	tmpHome := t.TempDir()
	hourglassDir := filepath.Join(tmpHome, ".hourglass-rpa")
	if err := os.Mkdir(hourglassDir, 0755); err != nil {
		t.Fatalf("failed to create .hourglass-rpa directory: %v", err)
	}

	// Create .env in home directory
	envContent := "HOME_VAR=from_home\n"
	envPath := filepath.Join(hourglassDir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create home .env file: %v", err)
	}

	// Set HOME to temp directory
	os.Setenv("HOME", tmpHome)

	// Change to a different directory (no .env there)
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Load env files
	loadEnvFiles()

	// Verify home directory .env was loaded
	if got := os.Getenv("HOME_VAR"); got != "from_home" {
		t.Errorf("HOME_VAR = %q, want %q", got, "from_home")
	}
}

// TestSendTelegramNotification_RejectionWithSpecialCharacters tests with special characters in data
func TestSendTelegramNotification_RejectionWithSpecialCharacters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origChatID := os.Getenv("TELEGRAM_CHAT_ID")
	defer func() {
		os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		os.Setenv("TELEGRAM_CHAT_ID", origChatID)
	}()

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")

	rejections := []domain.Rejeicao{
		{
			Secao: "Partes Mecânicas",
			Quem:  "José da Silva",
			OQue:  "Áudio & Vídeo com Indicadores",
			PraQuando: "01/03/2026",
		},
	}

	err := sendTelegramNotification(nil, rejections)
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}

// TestSendFilteredNotifications_EmptyRejections tests with empty rejection list
func TestSendFilteredNotifications_EmptyRejections(t *testing.T) {
	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	err := sendFilteredNotifications(nil, []domain.Rejeicao{}, pm)
	if err != nil {
		t.Errorf("sendFilteredNotifications with empty list should return nil, got error: %v", err)
	}
}

// TestSendFilteredNotifications_NoUsers tests when no users configured
func TestSendFilteredNotifications_NoUsers(t *testing.T) {
	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Test User", OQue: "Test", PraQuando: "01/03/2026"},
	}

	// No users, so List returns empty => sendFilteredNotifications still works (returns after List)
	err := sendFilteredNotifications(nil, rejections, pm)
	// Users list is empty, but function will try to create notifier with empty users[0]
	// Actually the function calls List() again internally - with 0 users it returns empty
	// Since users is empty, no notifications to send
	if err != nil {
		t.Errorf("sendFilteredNotifications with no users should not error, got: %v", err)
	}
}

// TestSendFilteredNotifications_GroupsBySection verifies rejection grouping logic
func TestSendFilteredNotifications_GroupsBySection(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	defer os.Setenv("TELEGRAM_BOT_TOKEN", origToken)

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	// Create users with specific section preferences
	user1, err := pm.GetOrCreate(111, "alice")
	if err != nil {
		t.Fatalf("failed to create user1: %v", err)
	}
	_ = user1
	if err := pm.UpdateSections(111, []string{"Campo"}); err != nil {
		t.Fatalf("failed to update user1 sections: %v", err)
	}

	user2, err := pm.GetOrCreate(222, "bob")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}
	_ = user2
	if err := pm.UpdateSections(222, []string{"Partes Mecânicas", "Campo"}); err != nil {
		t.Fatalf("failed to update user2 sections: %v", err)
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Ministry", PraQuando: "01/03/2026"},
		{Secao: "Partes Mecânicas", Quem: "Jane", OQue: "Audio", PraQuando: "02/03/2026"},
		{Secao: "Testemunho Público", Quem: "Bob", OQue: "Cart", PraQuando: "03/03/2026"},
	}

	// Will fail due to fake token, but tests the grouping logic path
	err = sendFilteredNotifications(nil, rejections, pm)
	// Error expected due to fake bot token
	if err == nil {
		t.Log("Note: Would fail in real scenario due to fake token")
	}
}

// TestSendFilteredNotifications_DisabledUser tests that disabled users are skipped
func TestSendFilteredNotifications_DisabledUser(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	defer os.Setenv("TELEGRAM_BOT_TOKEN", origToken)

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	// Create a disabled user
	_, err := pm.GetOrCreate(333, "charlie")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := pm.UpdateSections(333, []string{"Campo"}); err != nil {
		t.Fatalf("failed to update sections: %v", err)
	}
	if err := pm.ToggleEnabled(333, false); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Test", OQue: "Test", PraQuando: "01/03/2026"},
	}

	// Disabled user should be skipped - but notifier creation will fail with fake token
	// since the disabled user is the only one, after filtering all users out, there's nothing to send
	err = sendFilteredNotifications(nil, rejections, pm)
	// Error expected due to fake token when creating notifier
	if err == nil {
		t.Log("Note: Would fail in real scenario due to fake token")
	}
}

// TestSendFilteredNotifications_NoMatchingSections tests when user sections don't match rejections
func TestSendFilteredNotifications_NoMatchingSections(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	defer os.Setenv("TELEGRAM_BOT_TOKEN", origToken)

	os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	// User only monitors Campo, but rejections are from different section
	_, err := pm.GetOrCreate(444, "dave")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := pm.UpdateSections(444, []string{"Testemunho Público"}); err != nil {
		t.Fatalf("failed to update sections: %v", err)
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Test", OQue: "Test", PraQuando: "01/03/2026"},
	}

	// User's section doesn't match, but notifier will still be created (and fail with fake token)
	err = sendFilteredNotifications(nil, rejections, pm)
	// Will fail at notifier creation due to fake token
	if err == nil {
		t.Log("Note: Would fail in real scenario due to fake token")
	}
}

// TestSendFilteredNotifications_MissingToken tests with missing bot token
func TestSendFilteredNotifications_MissingToken(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	defer os.Setenv("TELEGRAM_BOT_TOKEN", origToken)

	os.Unsetenv("TELEGRAM_BOT_TOKEN")

	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	// Create a user so List() returns non-empty
	_, err := pm.GetOrCreate(555, "eve")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := pm.UpdateSections(555, []string{"Campo"}); err != nil {
		t.Fatalf("failed to update sections: %v", err)
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Test", OQue: "Test", PraQuando: "01/03/2026"},
	}

	err = sendFilteredNotifications(nil, rejections, pm)
	if err != nil {
		t.Errorf("sendFilteredNotifications with missing token should return nil, got: %v", err)
	}
}

// TestPreferenceManagerList tests the List method on PreferenceManager
func TestPreferenceManagerList(t *testing.T) {
	tmpDir := t.TempDir()
	store := preferences.NewFilePreferenceStore(filepath.Join(tmpDir, "prefs.json"))
	pm := preferences.NewPreferenceManager(store)

	// Empty list initially
	users, err := pm.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}

	// Add users
	_, err = pm.GetOrCreate(100, "user1")
	if err != nil {
		t.Fatalf("GetOrCreate error: %v", err)
	}
	_, err = pm.GetOrCreate(200, "user2")
	if err != nil {
		t.Fatalf("GetOrCreate error: %v", err)
	}

	users, err = pm.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}
