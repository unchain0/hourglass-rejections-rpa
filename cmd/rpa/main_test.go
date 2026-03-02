package main

import (
	"context"
	"os"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/api"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/sentry"
	"hourglass-rejections-rpa/internal/storage"
)

func TestLoadEnvFiles_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	loadEnvFiles()
}

func TestLoadEnvFiles_CurrentDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	envContent := "TEST_VAR_1=value1\nTEST_VAR_2=value2\n"
	if err := os.WriteFile(".env", []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create .env file: %v", err)
	}

	loadEnvFiles()

	if got := os.Getenv("TEST_VAR_1"); got != "value1" {
		t.Errorf("TEST_VAR_1 = %q, want %q", got, "value1")
	}
	if got := os.Getenv("TEST_VAR_2"); got != "value2" {
		t.Errorf("TEST_VAR_2 = %q, want %q", got, "value2")
	}
}

func TestLoadEnvFiles_InvalidHomeDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	origWd, _ := os.Getwd()
	defer func() {
		os.Setenv("HOME", origHome)
		os.Chdir(origWd)
	}()

	os.Setenv("HOME", "")

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	loadEnvFiles()
}

func TestParseChatID_ValidID(t *testing.T) {
	id, err := parseChatID("123456789")
	if err != nil {
		t.Errorf("parseChatID with valid ID should not error, got: %v", err)
	}
	if id != 123456789 {
		t.Errorf("parseChatID = %d, want %d", id, 123456789)
	}
}

func TestParseChatID_InvalidID(t *testing.T) {
	_, err := parseChatID("invalid")
	if err == nil {
		t.Error("parseChatID with invalid ID should return error")
	}
}

func TestParseChatID_EmptyString(t *testing.T) {
	_, err := parseChatID("")
	if err == nil {
		t.Error("parseChatID with empty string should return error")
	}
}

func TestParseChatID_NegativeNumber(t *testing.T) {
	id, err := parseChatID("-123456789")
	if err != nil {
		t.Errorf("parseChatID with negative number should not error, got: %v", err)
	}
	if id != -123456789 {
		t.Errorf("parseChatID = %d, want %d", id, -123456789)
	}
}

func TestParseWhitelist_ValidIDs(t *testing.T) {
	ids := parseWhitelist("123,456,789")
	if len(ids) != 3 {
		t.Errorf("parseWhitelist returned %d IDs, want 3", len(ids))
	}
	if ids[0] != 123 || ids[1] != 456 || ids[2] != 789 {
		t.Errorf("parseWhitelist returned wrong IDs: %v", ids)
	}
}

func TestParseWhitelist_WithWhitespace(t *testing.T) {
	ids := parseWhitelist(" 123 , 456 , 789 ")
	if len(ids) != 3 {
		t.Errorf("parseWhitelist returned %d IDs, want 3", len(ids))
	}
	if ids[0] != 123 || ids[1] != 456 || ids[2] != 789 {
		t.Errorf("parseWhitelist returned wrong IDs: %v", ids)
	}
}

func TestParseWhitelist_InvalidIDs(t *testing.T) {
	ids := parseWhitelist("123,invalid,456")
	if len(ids) != 2 {
		t.Errorf("parseWhitelist returned %d IDs, want 2", len(ids))
	}
	if ids[0] != 123 || ids[1] != 456 {
		t.Errorf("parseWhitelist returned wrong IDs: %v", ids)
	}
}

func TestParseWhitelist_EmptyString(t *testing.T) {
	ids := parseWhitelist("")
	if len(ids) != 0 {
		t.Errorf("parseWhitelist returned %d IDs, want 0", len(ids))
	}
}

func TestParseWhitelist_SingleID(t *testing.T) {
	ids := parseWhitelist("123456789")
	if len(ids) != 1 {
		t.Errorf("parseWhitelist returned %d IDs, want 1", len(ids))
	}
	if ids[0] != 123456789 {
		t.Errorf("parseWhitelist returned wrong ID: %d", ids[0])
	}
}

func TestParseWhitelist_AllInvalid(t *testing.T) {
	ids := parseWhitelist("invalid,also-invalid,nope")
	if len(ids) != 0 {
		t.Errorf("parseWhitelist returned %d IDs, want 0", len(ids))
	}
}

func TestRun_InvalidArgs(t *testing.T) {
	opts := runOptions{
		args:   []string{"-invalid-flag"},
		getenv: func(string) string { return "" },
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err == nil {
		t.Error("expected error for invalid args")
	}
}

func TestRun_HelpFlag(t *testing.T) {
	opts := runOptions{
		args:   []string{"-h"},
		getenv: func(string) string { return "" },
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err == nil {
		t.Error("expected error for help flag")
	}
}

func TestRun_OnceMode(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	envContent := `HOURGLASS_XSRF_TOKEN=test-token
HOURGLASS_HGLOGIN_COOKIE=test-cookie
OUTPUT_DIR=/tmp
`
	os.WriteFile(".env", []byte(envContent), 0644)

	opts := runOptions{
		args:   []string{"-once"},
		getenv: func(s string) string { return "" },
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err == nil {
		t.Error("expected error because runOnce is not implemented")
	}
}

func TestSetupLogging(t *testing.T) {
	setupLogging("debug")
	setupLogging("")
	setupLogging("info")
}

func TestSetupSentry(t *testing.T) {
	cfg := &config.Config{}
	client := setupSentry(cfg)
	if client == nil {
		t.Error("setupSentry should return a client")
	}
}

func TestSetupSentry_WithDSN(t *testing.T) {
	cfg := &config.Config{
		SentryDSN:         "https://test@sentry.io/123",
		SentryEnvironment: "test",
	}
	client := setupSentry(cfg)
	if client == nil {
		t.Error("setupSentry should return a client")
	}
}

func TestSetupDependencies(t *testing.T) {
	cfg := &config.Config{
		HourglassXSRFToken: "test-xsrf",
		HourglassHGLogin:   "test-login",
	}

	analyzer, store := setupDependencies(cfg)
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestSetupDependencies_NoTokens(t *testing.T) {
	cfg := &config.Config{}

	analyzer, store := setupDependencies(cfg)
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestRunOnceMode(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	apiClient := api.NewClient()
	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runOnceMode(ctx, cfg, sentryClient, analyzer, store)
	if err == nil {
		t.Error("expected error because runOnce is not implemented")
	}
}
