package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadEnvFiles_GodotenvLoadSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	envDir := filepath.Join(tmpDir, "sub")
	os.MkdirAll(envDir, 0755)

	if err := os.Chdir(envDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	envContent := "LOAD_SUCCESS_TEST=yes\n"
	if err := os.WriteFile(filepath.Join(envDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to create .env: %v", err)
	}

	loadEnvFiles()

	if got := os.Getenv("LOAD_SUCCESS_TEST"); got != "yes" {
		t.Errorf("LOAD_SUCCESS_TEST = %q, want %q", got, "yes")
	}
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

func TestRun_FullMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := runOptions{
		args:   []string{},
		getenv: func(s string) string { return "" },
		exit:   func(int) {},
	}

	err := run(ctx, opts)
	if err != nil {
		t.Errorf("expected no error for full mode with cancelled context, got: %v", err)
	}
}

func TestRun_OnceModeSuccess(t *testing.T) {
	origFn := runOnceFn
	defer func() { runOnceFn = origFn }()

	runOnceFn = func(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
		return nil
	}

	opts := runOptions{
		args:   []string{"-once"},
		getenv: func(s string) string { return "" },
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestMain_WithError(t *testing.T) {
	origArgs := os.Args
	origExit := osExit
	defer func() {
		os.Args = origArgs
		osExit = origExit
	}()

	os.Args = []string{"rpa", "-invalid-flag-for-test"}

	var exitCode int
	exitCalled := false
	osExit = func(code int) {
		exitCode = code
		exitCalled = true
	}

	main()

	if !exitCalled {
		t.Error("expected exit to be called")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestMain_Success(t *testing.T) {
	origFn := runOnceFn
	origArgs := os.Args
	origExit := osExit
	defer func() {
		runOnceFn = origFn
		os.Args = origArgs
		osExit = origExit
	}()

	runOnceFn = func(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
		return nil
	}

	os.Args = []string{"rpa", "-once"}

	exitCalled := false
	osExit = func(code int) {
		exitCalled = true
	}

	main()

	if exitCalled {
		t.Error("expected exit not to be called on success")
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

func TestRunOnceMode_Success(t *testing.T) {
	origFn := runOnceFn
	defer func() { runOnceFn = origFn }()

	runOnceFn = func(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
		return nil
	}

	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	apiClient := api.NewClient()
	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)

	err := runOnceMode(context.Background(), cfg, sentryClient, analyzer, store)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRunFullMode_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	apiClient := api.NewClient()
	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)

	err := runFullMode(ctx, cfg, sentryClient, analyzer, store)
	if err != nil {
		t.Errorf("expected no error with cancelled context, got: %v", err)
	}
}

func TestRunFullMode_WithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	apiClient := api.NewClient()
	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)

	err := runFullMode(ctx, cfg, sentryClient, analyzer, store)
	if err != nil {
		t.Errorf("expected no error when context times out, got: %v", err)
	}
}

func TestRun_ConfigLoadError(t *testing.T) {
	t.Setenv("TIMEOUT", "not-a-duration")

	opts := runOptions{
		args:   []string{"-once"},
		getenv: os.Getenv,
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestRun_SentryEnabled(t *testing.T) {
	origFn := runOnceFn
	defer func() { runOnceFn = origFn }()

	runOnceFn = func(ctx context.Context, cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) error {
		return nil
	}

	t.Setenv("SENTRY_DSN", "https://examplePublicKey@o0.ingest.sentry.io/0")
	t.Setenv("SENTRY_ENVIRONMENT", "test")

	opts := runOptions{
		args:   []string{"-once"},
		getenv: os.Getenv,
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

type errorRunner struct {
	err error
}

func (r *errorRunner) Run(ctx context.Context) error {
	return r.err
}

func TestRunFullMode_SchedulerError(t *testing.T) {
	origFn := newSchedulerFn
	defer func() { newSchedulerFn = origFn }()

	newSchedulerFn = func(cfg *config.Config, sentryClient *sentry.Client, analyzer *api.APIAnalyzer, store *storage.FileStorage) runner {
		return &errorRunner{err: fmt.Errorf("mock scheduler failure")}
	}

	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	apiClient := api.NewClient()
	analyzer := api.NewAPIAnalyzer(apiClient)
	store := storage.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runFullMode(ctx, cfg, sentryClient, analyzer, store)
	if err == nil {
		t.Fatal("expected error from scheduler")
	}
	if !strings.Contains(err.Error(), "scheduler failed") {
		t.Errorf("expected 'scheduler failed' in error, got: %v", err)
	}
}
