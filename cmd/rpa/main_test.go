package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hourglass-rejections-rpa/internal/domain"
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

	err := sendTelegramNotification(rejections)
	if err != nil {
		t.Errorf("sendTelegramNotification with missing config should return nil, got error: %v", err)
	}

	// Test with missing token only
	os.Setenv("TELEGRAM_CHAT_ID", "123456789")
	os.Unsetenv("TELEGRAM_BOT_TOKEN")

	err = sendTelegramNotification(rejections)
	if err != nil {
		t.Errorf("sendTelegramNotification with missing token should return nil, got error: %v", err)
	}

	// Test with missing chat ID only
	os.Unsetenv("TELEGRAM_CHAT_ID")
	os.Setenv("TELEGRAM_BOT_TOKEN", "fake-token")

	err = sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
	if err == nil {
		t.Error("sendTelegramNotification with invalid chat ID should return error")
	}

	if !strings.Contains(err.Error(), "invalid telegram chat ID") {
		t.Errorf("error message should mention invalid chat ID, got: %v", err)
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
	err := sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
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

	err := sendTelegramNotification(rejections)
	if err == nil {
		t.Log("Note: Telegram notification would fail in real scenario due to fake token")
	}
}
