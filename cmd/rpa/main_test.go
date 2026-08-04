package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"
	hourglass "hourglass-rejections-rpa/src/services/hourglass"
)

func newTestStore(t *testing.T) *preferences.Store {
	t.Helper()

	store, err := preferences.NewStore(filepath.Join(t.TempDir(), "hourglass.db"))
	require.NoError(t, err)
	return store
}

func TestMain(m *testing.M) {
	origNewPreferenceStore := newPreferenceStore
	newPreferenceStore = func(_ *config.Config) (*preferences.Store, error) {
		return preferences.NewStore(filepath.Join(os.TempDir(), fmt.Sprintf("hourglass-main-test-%d.db", time.Now().UnixNano())))
	}
	defer func() { newPreferenceStore = origNewPreferenceStore }()

	os.Exit(m.Run())
}

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
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

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
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

	opts := runOptions{
		args:   []string{"-h"},
		getenv: func(string) string { return "" },
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	if err != nil {
		t.Errorf("expected no error for help flag, got: %v", err)
	}
}

func TestRun_OnceMode(t *testing.T) {
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	envContent := `HOURGLASS_XSRF_TOKEN=test-token
HOURGLASS_HGLOGIN_COOKIE=test-cookie
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
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

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
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

	origFn := runOnceFn
	defer func() { runOnceFn = origFn }()

	runOnceFn = func(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
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
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

	origFn := runOnceFn
	origArgs := os.Args
	origExit := osExit
	defer func() {
		runOnceFn = origFn
		os.Args = origArgs
		osExit = origExit
	}()

	runOnceFn = func(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
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

func TestSetupTelemetry(t *testing.T) {
	cfg := &config.Config{}
	client := setupTelemetry(cfg)
	if client == nil {
		t.Error("setupTelemetry should return a client")
	}
}

func TestSetupTelemetry_WithEndpoint(t *testing.T) {
	cfg := &config.Config{
		OTLPEndpoint:            "https://otel.example.com",
		OTLPHeaders:             "Authorization=Bearer token,stream-name=default",
		DeploymentEnvironment:   "test",
		TelemetryServiceName:    "hourglass-rejections-rpa",
		TelemetryServiceVersion: "1.0.0",
		OTLPMetricInterval:      time.Second,
	}
	client := setupTelemetry(cfg)
	if client == nil {
		t.Error("setupTelemetry should return a client")
	}
}

func TestSetupDependencies(t *testing.T) {
	cfg := &config.Config{
		HourglassXSRFToken: "test-xsrf",
		HourglassHGLogin:   "test-login",
	}

	apiClient, analyzer, store, err := setupDependencies(cfg)
	require.NoError(t, err)
	if apiClient == nil {
		t.Error("setupDependencies should return an apiClient")
	}
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestSetupDependencies_NoTokens(t *testing.T) {
	cfg := &config.Config{}

	apiClient, analyzer, store, err := setupDependencies(cfg)
	require.NoError(t, err)
	if apiClient == nil {
		t.Error("setupDependencies should return an apiClient")
	}
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestSetupDependencies_WithTokensPathEnv(t *testing.T) {
	tmpDir := t.TempDir()
	tokensPath := filepath.Join(tmpDir, "auth-tokens.json")

	// Create a valid tokens file
	tokens := `{"xsrf_token":"test-token","hg_login":"test-login","expires_at":"2099-01-01T00:00:00Z"}`
	if err := os.WriteFile(tokensPath, []byte(tokens), 0644); err != nil {
		t.Fatalf("failed to create tokens file: %v", err)
	}

	origEnv := os.Getenv("TOKENS_PATH")
	os.Setenv("TOKENS_PATH", tokensPath)
	defer os.Setenv("TOKENS_PATH", origEnv)

	cfg := &config.Config{}

	apiClient, analyzer, store, err := setupDependencies(cfg)
	require.NoError(t, err)
	if apiClient == nil {
		t.Error("setupDependencies should return an apiClient")
	}
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestNewConfiguredAPIClient_AppliesAuthPaths(t *testing.T) {
	cfg := &config.Config{HourglassURL: "https://example.com"}
	client := newConfiguredAPIClient(cfg, authPaths{
		tokensPath:        "/tmp/auth-tokens.json",
		browserProfileDir: "/tmp/chrome-profile",
	})

	assert.NotNil(t, client)
}

func TestSetupDependencies_NoHomeDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	os.Unsetenv("HOME")
	defer os.Setenv("HOME", origHome)

	origUserProfile := os.Getenv("USERPROFILE")
	os.Unsetenv("USERPROFILE")
	defer os.Setenv("USERPROFILE", origUserProfile)

	origHomeDrive := os.Getenv("HOMEDRIVE")
	origHomePath := os.Getenv("HOMEPATH")
	os.Unsetenv("HOMEDRIVE")
	os.Unsetenv("HOMEPATH")
	defer func() {
		if origHomeDrive != "" {
			os.Setenv("HOMEDRIVE", origHomeDrive)
		}
		if origHomePath != "" {
			os.Setenv("HOMEPATH", origHomePath)
		}
	}()

	cfg := &config.Config{}

	apiClient, analyzer, store, err := setupDependencies(cfg)
	require.NoError(t, err)
	if apiClient == nil {
		t.Error("setupDependencies should return an apiClient even without home dir")
	}
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestSetupDependencies_InvalidTokensFile(t *testing.T) {
	tmpDir := t.TempDir()
	invalidTokensPath := filepath.Join(tmpDir, "invalid-tokens.json")

	if err := os.WriteFile(invalidTokensPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("failed to create invalid tokens file: %v", err)
	}

	origEnv := os.Getenv("TOKENS_PATH")
	os.Setenv("TOKENS_PATH", invalidTokensPath)
	defer os.Setenv("TOKENS_PATH", origEnv)

	cfg := &config.Config{}

	apiClient, analyzer, store, err := setupDependencies(cfg)
	require.NoError(t, err)
	if apiClient == nil {
		t.Error("setupDependencies should return an apiClient even with invalid tokens file")
	}
	if analyzer == nil {
		t.Error("setupDependencies should return an analyzer")
	}
	if store == nil {
		t.Error("setupDependencies should return a store")
	}
}

func TestSetupDependencies_EnableWebAuthnTokenManager(t *testing.T) {
	tmpDir := t.TempDir()
	credentialsPath := filepath.Join(tmpDir, "webauthn-credentials.json")
	require.NoError(t, os.WriteFile(credentialsPath, []byte("{}"), 0o600))

	cfg := &config.Config{
		AutoRefreshTokens:       true,
		WebAuthnCredentialsPath: credentialsPath,
		TokensPath:              filepath.Join(tmpDir, "auth-tokens.json"),
	}

	apiClient, analyzer, store, err := setupDependencies(cfg)
	require.NoError(t, err)
	require.NotNil(t, apiClient)
	require.NotNil(t, analyzer)
	require.NotNil(t, store)
}

func TestResolveTokensPath(t *testing.T) {
	t.Run("uses config path", func(t *testing.T) {
		cfg := &config.Config{TokensPath: "/tmp/from-config.json"}
		assert.Equal(t, "/tmp/from-config.json", resolveTokensPath(cfg))
	})

	t.Run("uses env tokens path", func(t *testing.T) {
		t.Setenv("TOKENS_PATH", "/tmp/from-env.json")
		cfg := &config.Config{}
		assert.Equal(t, "/tmp/from-env.json", resolveTokensPath(cfg))
	})

	t.Run("uses webauthn env tokens path", func(t *testing.T) {
		t.Setenv("WEBAUTHN_TOKENS_PATH", "/tmp/from-webauthn-env.json")
		cfg := &config.Config{}
		assert.Equal(t, "/tmp/from-webauthn-env.json", resolveTokensPath(cfg))
	})
}

func TestResolveWebAuthnCredentialsPath(t *testing.T) {
	t.Run("uses config path", func(t *testing.T) {
		cfg := &config.Config{WebAuthnCredentialsPath: "/tmp/from-config.json"}
		assert.Equal(t, "/tmp/from-config.json", resolveWebAuthnCredentialsPath(cfg))
	})

	t.Run("uses env path", func(t *testing.T) {
		t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "/tmp/from-env.json")
		cfg := &config.Config{}
		assert.Equal(t, "/tmp/from-env.json", resolveWebAuthnCredentialsPath(cfg))
	})

	t.Run("falls back to home directory", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
		cfg := &config.Config{}
		assert.Equal(t, filepath.Join(homeDir, ".hourglass-rpa", "webauthn-credentials.json"), resolveWebAuthnCredentialsPath(cfg))
	})

	t.Run("returns empty when home is unavailable", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
		cfg := &config.Config{}
		assert.Empty(t, resolveWebAuthnCredentialsPath(cfg))
	})
}

func TestResolveChromeProfileDir(t *testing.T) {
	t.Run("uses config path", func(t *testing.T) {
		cfg := &config.Config{ChromeProfileDir: "/tmp/from-config-profile"}
		assert.Equal(t, "/tmp/from-config-profile", resolveChromeProfileDir(cfg))
	})

	t.Run("uses env path", func(t *testing.T) {
		t.Setenv("CHROME_PROFILE_DIR", "/tmp/from-env-profile")
		cfg := &config.Config{}
		assert.Equal(t, "/tmp/from-env-profile", resolveChromeProfileDir(cfg))
	})

	t.Run("returns empty when unset", func(t *testing.T) {
		t.Setenv("CHROME_PROFILE_DIR", "")
		cfg := &config.Config{}
		assert.Empty(t, resolveChromeProfileDir(cfg))
	})
}

func TestEnableWebAuthnTokenManager(t *testing.T) {
	t.Run("disabled by config", func(t *testing.T) {
		client := hourglass.NewClient()
		cfg := &config.Config{AutoRefreshTokens: false}
		assert.False(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("credentials missing", func(t *testing.T) {
		client := hourglass.NewClient()
		cfg := &config.Config{
			AutoRefreshTokens:       true,
			WebAuthnCredentialsPath: filepath.Join(t.TempDir(), "missing.json"),
		}
		assert.False(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("credentials path unavailable", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")

		client := hourglass.NewClient()
		cfg := &config.Config{AutoRefreshTokens: true}
		assert.False(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("credentials path stat error", func(t *testing.T) {
		client := hourglass.NewClient()
		cfg := &config.Config{
			AutoRefreshTokens:       true,
			WebAuthnCredentialsPath: string([]byte{0}),
		}
		assert.False(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("credentials stat error still enables with chrome profile fallback", func(t *testing.T) {
		origEnable := enableWebAuthnClient
		enableWebAuthnClient = func(apiClient *hourglass.Client, credentialsPath string) error {
			return nil
		}
		defer func() { enableWebAuthnClient = origEnable }()

		client := hourglass.NewClient()
		cfg := &config.Config{
			AutoRefreshTokens:       true,
			WebAuthnCredentialsPath: string([]byte{0}),
			ChromeProfileDir:        filepath.Join(t.TempDir(), "chrome-profile"),
		}

		assert.True(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("enable success", func(t *testing.T) {
		tmpDir := t.TempDir()
		credentialsPath := filepath.Join(tmpDir, "webauthn-credentials.json")
		require.NoError(t, os.WriteFile(credentialsPath, []byte("{}"), 0o600))

		client := hourglass.NewClient()
		client.SetWebAuthnTokensPath(filepath.Join(tmpDir, "auth-tokens.json"))
		cfg := &config.Config{
			AutoRefreshTokens:       true,
			WebAuthnCredentialsPath: credentialsPath,
		}
		assert.True(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("enables with chrome profile even without credentials", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		tmpDir := t.TempDir()
		client := hourglass.NewClient()
		client.SetWebAuthnTokensPath(filepath.Join(tmpDir, "auth-tokens.json"))
		cfg := &config.Config{
			AutoRefreshTokens: true,
			ChromeProfileDir:  filepath.Join(tmpDir, "chrome-profile"),
		}

		assert.True(t, enableWebAuthnTokenManager(client, cfg))
	})

	t.Run("enable failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		credentialsPath := filepath.Join(tmpDir, "webauthn-credentials.json")
		require.NoError(t, os.WriteFile(credentialsPath, []byte("{}"), 0o600))

		origEnable := enableWebAuthnClient
		enableWebAuthnClient = func(apiClient *hourglass.Client, credentialsPath string) error {
			return fmt.Errorf("boom")
		}
		defer func() { enableWebAuthnClient = origEnable }()

		client := hourglass.NewClient()
		cfg := &config.Config{
			AutoRefreshTokens:       true,
			WebAuthnCredentialsPath: credentialsPath,
		}
		assert.False(t, enableWebAuthnTokenManager(client, cfg))
	})
}

func TestRun_TokenManagerStartError(t *testing.T) {
	t.Setenv("AUTO_REFRESH_TOKENS", "true")
	tmpDir := t.TempDir()
	credentialsPath := filepath.Join(tmpDir, "webauthn-credentials.json")
	require.NoError(t, os.WriteFile(credentialsPath, []byte("{}"), 0o600))

	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", credentialsPath)
	t.Setenv("WEBAUTHN_TOKENS_PATH", tmpDir)

	opts := runOptions{
		args:   []string{"-once"},
		getenv: os.Getenv,
		exit:   func(int) {},
	}

	err := run(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start token manager")
}

func TestRunOnceMode(t *testing.T) {
	cfg := &config.Config{}
	telemetryClient := &telemetry.Client{}
	apiClient := hourglass.NewClient()
	analyzer := hourglass.NewAPIAnalyzer(apiClient)
	store := newTestStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runOnceMode(ctx, cfg, telemetryClient, analyzer, store)
	if err == nil {
		t.Error("expected error because runOnce is not implemented")
	}
}

func TestRunOnceMode_Success(t *testing.T) {
	origFn := runOnceFn
	defer func() { runOnceFn = origFn }()

	runOnceFn = func(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
		return nil
	}

	cfg := &config.Config{}
	telemetryClient := &telemetry.Client{}
	apiClient := hourglass.NewClient()
	analyzer := hourglass.NewAPIAnalyzer(apiClient)
	store := newTestStore(t)

	err := runOnceMode(context.Background(), cfg, telemetryClient, analyzer, store)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRunFullMode_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{}
	telemetryClient := &telemetry.Client{}
	apiClient := hourglass.NewClient()
	analyzer := hourglass.NewAPIAnalyzer(apiClient)
	store := newTestStore(t)

	err := runFullMode(ctx, cfg, telemetryClient, analyzer, store)
	if err != nil {
		t.Errorf("expected no error with cancelled context, got: %v", err)
	}
}

func TestRunFullMode_WithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := &config.Config{}
	telemetryClient := &telemetry.Client{}
	apiClient := hourglass.NewClient()
	analyzer := hourglass.NewAPIAnalyzer(apiClient)
	store := newTestStore(t)

	err := runFullMode(ctx, cfg, telemetryClient, analyzer, store)
	if err != nil {
		t.Errorf("expected no error when context times out, got: %v", err)
	}
}

func TestRun_ConfigLoadError(t *testing.T) {
	t.Setenv("TIMEOUT", "not-a-duration")
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

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

func TestRun_TelemetryEnabled(t *testing.T) {
	t.Setenv("AUTO_REFRESH_TOKENS", "false")

	origFn := runOnceFn
	defer func() { runOnceFn = origFn }()

	runOnceFn = func(ctx context.Context, cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) error {
		return nil
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://otel.example.com")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer token,stream-name=default")
	t.Setenv("DEPLOYMENT_ENVIRONMENT", "test")

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

	newSchedulerFn = func(cfg *config.Config, telemetryClient *telemetry.Client, analyzer *hourglass.APIAnalyzer, store *preferences.Store) runner {
		return &errorRunner{err: fmt.Errorf("mock scheduler failure")}
	}

	cfg := &config.Config{}
	telemetryClient := &telemetry.Client{}
	apiClient := hourglass.NewClient()
	analyzer := hourglass.NewAPIAnalyzer(apiClient)
	store := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runFullMode(ctx, cfg, telemetryClient, analyzer, store)
	if err == nil {
		t.Fatal("expected error from scheduler")
	}
	if !strings.Contains(err.Error(), "scheduler failed") {
		t.Errorf("expected 'scheduler failed' in error, got: %v", err)
	}
}

func TestCaptureError(t *testing.T) {
	t.Run("with nil telemetry client", func(t *testing.T) {
		telemetryClientGlobal = nil
		assert.NotPanics(t, func() {
			captureError(fmt.Errorf("test error"), nil)
		})
	})

	t.Run("with disabled telemetry client", func(t *testing.T) {
		telemetryClientGlobal = &telemetry.Client{}
		assert.NotPanics(t, func() {
			captureError(fmt.Errorf("test error"), nil)
		})
	})

	t.Run("with enabled telemetry client", func(t *testing.T) {
		mockClient := &telemetry.Client{}
		telemetryClientGlobal = mockClient
		assert.NotPanics(t, func() {
			captureError(fmt.Errorf("test error"), map[string]interface{}{"key": "value"})
		})
	})

	// Test with actually enabled telemetry client
	t.Run("with truly enabled telemetry client", func(t *testing.T) {
		enabledClient, err := telemetry.New(telemetry.Config{
			Endpoint:       "https://otel.example.com",
			Headers:        "Authorization=Bearer token,stream-name=default",
			Environment:    "test",
			ServiceName:    "hourglass-rejections-rpa",
			Release:        "1.0.0",
			MetricInterval: time.Second,
		})
		if err != nil {
			t.Skipf("Skipping test: could not create enabled telemetry client: %v", err)
		}
		defer enabledClient.Close()

		telemetryClientGlobal = enabledClient
		assert.NotPanics(t, func() {
			captureError(fmt.Errorf("test error with enabled telemetry"), map[string]interface{}{"test": "data"})
		})
	})
}
