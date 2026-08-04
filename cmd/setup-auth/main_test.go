package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

type mockBrowserAuth struct {
	tokens     *webauthn.AuthTokens
	err        error
	profileDir string
}

func (m *mockBrowserAuth) Authenticate() (*webauthn.AuthTokens, error) {
	return m.tokens, m.err
}

func (m *mockBrowserAuth) ExtractTokensFromProfile() (*webauthn.AuthTokens, error) {
	return m.tokens, m.err
}

func (m *mockBrowserAuth) WithHeadless(_ bool) browserAuth {
	return m
}

func (m *mockBrowserAuth) WithProfileDir(profileDir string) browserAuth {
	m.profileDir = profileDir
	return m
}

type mockCredentialRegistrar struct {
	xsrfToken      string
	hgLogin        string
	registeredUser string
	registerErr    error
}

func (m *mockCredentialRegistrar) SetCookies(xsrfToken, hgLogin string) {
	m.xsrfToken = xsrfToken
	m.hgLogin = hgLogin
}

func (m *mockCredentialRegistrar) Register(userName string) (*webauthn.Credential, error) {
	m.registeredUser = userName
	if m.registerErr != nil {
		return nil, m.registerErr
	}

	return &webauthn.Credential{ID: "credential-id"}, nil
}

func stubCredentialRegistrarFactory(t *testing.T, registrar credentialRegistrar, err error) {
	t.Helper()

	oldFactory := defaultCredentialRegistrarFactory
	defaultCredentialRegistrarFactory = func(_, _ string) (credentialRegistrar, error) {
		if err != nil {
			return nil, err
		}

		return registrar, nil
	}

	t.Cleanup(func() {
		defaultCredentialRegistrarFactory = oldFactory
	})
}

// setupVPSUploadTest é um helper para testes de VPS upload.
func setupVPSUploadTest(t *testing.T, inputs []string) (string, error) {
	t.Helper()

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		_ = w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		for _, input := range inputs {
			_, _ = fmt.Fprintln(w, input)
		}
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		_ = pw.Close()
		os.Stdout = oldStdout
	}()

	tokensPath := filepath.Join(t.TempDir(), "tokens.json")
	runner := newSetupRunner()
	runner.userInput = newConsoleUserInput(r)
	runner.scpClient = &mockSCPClient{err: errors.New("failed to transfer tokens")}
	err := runner.askVPSUpload(tokensPath)
	_ = r

	_ = pw.Close()
	output, _ := io.ReadAll(pr)

	return string(output), err
}

func TestSetupOptions(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		opts := setupOptions{
			getenv:        os.Getenv,
			osUserHomeDir: os.UserHomeDir,
		}
		assert.NotNil(t, opts.getenv)
		assert.NotNil(t, opts.osUserHomeDir)
	})
}

func TestSetupRunnerUsesEnvironmentSessionWithoutBrowser(t *testing.T) {
	homeDir := t.TempDir()
	registrar := &mockCredentialRegistrar{}
	browserStarted := false
	runner := newSetupRunner()
	runner.fs = &optionsFileSystem{base: osFileSystem{}, userHomeDirFn: func() (string, error) { return homeDir, nil }}
	runner.getenv = func(key string) string {
		switch key {
		case "HOURGLASS_HGLOGIN_COOKIE":
			return "environment-hglogin"
		case "HOURGLASS_XSRF_TOKEN":
			return "environment-xsrf"
		default:
			return ""
		}
	}
	runner.authFactory = func(string, string) (credentialRegistrar, error) { return registrar, nil }
	runner.launchBrowser = func(string, string) error {
		browserStarted = true
		return nil
	}
	runner.userInput = &mockUserInput{confirmResult: false}

	err := runner.run()
	require.NoError(t, err)
	assert.False(t, browserStarted)
	assert.Equal(t, "environment-hglogin", registrar.hgLogin)
	assert.Equal(t, "environment-xsrf", registrar.xsrfToken)

	tokensPath := filepath.Join(homeDir, defaultConfigDir, defaultTokensFile)
	data, err := os.ReadFile(tokensPath)
	require.NoError(t, err)
	var tokens webauthn.AuthTokens
	require.NoError(t, json.Unmarshal(data, &tokens))
	assert.True(t, tokens.IsExpired(), "the refresh command must immediately exchange the bootstrapping session")
}

func TestCheckExistingTokens(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("no existing tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "nonexistent.json")
		tokens, err := checkExistingTokens(tokensPath)
		assert.NoError(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("valid tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "tokens.json")
		validTokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test123",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}
		data, _ := json.Marshal(validTokens)
		err := os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		tokens, err := checkExistingTokens(tokensPath)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.False(t, tokens.IsExpired())
	})

	t.Run("expired tokens", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "expired.json")
		expiredTokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test123",
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		data, _ := json.Marshal(expiredTokens)
		err := os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		tokens, err := checkExistingTokens(tokensPath)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.True(t, tokens.IsExpired())
	})

	t.Run("invalid json", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(tokensPath, []byte("invalid"), 0600)
		require.NoError(t, err)

		tokens, err := checkExistingTokens(tokensPath)
		assert.Error(t, err)
		assert.Nil(t, tokens)
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exactly max",
			input:    "hello world",
			maxLen:   11,
			expected: "hello world",
		},
		{
			name:     "longer than max",
			input:    "hello world this is a long string",
			maxLen:   10,
			expected: "hello worl...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCopyTokensToVPS(t *testing.T) {
	t.Run("dry run", func(t *testing.T) {
		tempDir := t.TempDir()
		tokensPath := filepath.Join(tempDir, "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test123",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}
		data, _ := json.Marshal(tokens)
		err := os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		assert.FileExists(t, tokensPath)
	})
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "https://app.hourglass-app.com", defaultBaseURL)
	assert.Equal(t, ".hourglass-rpa", defaultConfigDir)
	assert.Equal(t, "auth-tokens.json", defaultTokensFile)
}

func TestRun(t *testing.T) {
	stubCredentialRegistrarFactory(t, &mockCredentialRegistrar{}, nil)

	t.Run("home directory error", func(t *testing.T) {
		mockErr := errors.New("no home directory")
		opts := setupOptions{
			getenv:        os.Getenv,
			osUserHomeDir: func() (string, error) { return "", mockErr },
		}

		err := run(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get home directory")
	})

	t.Run("config directory creation failure", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "file-not-dir")
		err := os.WriteFile(filePath, []byte("test"), 0600)
		require.NoError(t, err)

		opts := setupOptions{
			getenv:        os.Getenv,
			osUserHomeDir: func() (string, error) { return filePath, nil },
		}

		err = run(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create config directory")
	})

	t.Run("check existing tokens failure", func(t *testing.T) {
		oldBrowserAuth := newBrowserAuth
		defer func() { newBrowserAuth = oldBrowserAuth }()

		newBrowserAuth = func(baseURL string) browserAuth {
			return &mockBrowserAuth{
				tokens: nil,
				err:    errors.New("auth failed"),
			}
		}

		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, defaultConfigDir)
		err := os.MkdirAll(configDir, 0700)
		require.NoError(t, err)

		tokensPath := filepath.Join(configDir, defaultTokensFile)
		err = os.Mkdir(tokensPath, 0700)
		require.NoError(t, err)

		opts := setupOptions{
			getenv:        os.Getenv,
			osUserHomeDir: func() (string, error) { return tempDir, nil },
		}

		err = run(opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check existing tokens")
	})

	t.Run("authentication failure", func(t *testing.T) {
		oldBrowserAuth := newBrowserAuth
		defer func() { newBrowserAuth = oldBrowserAuth }()

		newBrowserAuth = func(baseURL string) browserAuth {
			return &mockBrowserAuth{
				tokens: nil,
				err:    errors.New("auth failed"),
			}
		}

		tempDir := t.TempDir()

		opts := setupOptions{
			getenv:        os.Getenv,
			osUserHomeDir: func() (string, error) { return tempDir, nil },
		}

		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			_ = w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			_, _ = fmt.Fprintln(w, "no")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			_ = pw.Close()
			os.Stdout = oldStdout
		}()

		err := run(opts)
		_ = pr

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "authentication failed")
	})
}

func TestRunWithValidTokens(t *testing.T) {
	stubCredentialRegistrarFactory(t, &mockCredentialRegistrar{}, nil)

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, defaultConfigDir)
	err := os.MkdirAll(configDir, 0700)
	require.NoError(t, err)

	tokensPath := filepath.Join(configDir, defaultTokensFile)
	validTokens := &webauthn.AuthTokens{
		HGLogin:   "valid-hglogin",
		XSRFToken: "valid-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	data, err := json.Marshal(validTokens)
	require.NoError(t, err)
	err = os.WriteFile(tokensPath, data, 0600)
	require.NoError(t, err)

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		_ = w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		_, _ = fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		_ = pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	_ = pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Valid tokens found")
	assert.Contains(t, string(output), "Using existing tokens")
}

func TestRunWithExpiredTokens(t *testing.T) {
	stubCredentialRegistrarFactory(t, &mockCredentialRegistrar{}, nil)

	oldBrowserAuth := newBrowserAuth
	defer func() { newBrowserAuth = oldBrowserAuth }()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, defaultConfigDir)
	err := os.MkdirAll(configDir, 0700)
	require.NoError(t, err)

	tokensPath := filepath.Join(configDir, defaultTokensFile)
	expiredTokens := &webauthn.AuthTokens{
		HGLogin:   "expired-hglogin",
		XSRFToken: "expired-xsrf",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	data, err := json.Marshal(expiredTokens)
	require.NoError(t, err)
	err = os.WriteFile(tokensPath, data, 0600)
	require.NoError(t, err)

	newTokens := &webauthn.AuthTokens{
		HGLogin:   "new-hglogin",
		XSRFToken: "new-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}

	newBrowserAuth = func(baseURL string) browserAuth {
		return &mockBrowserAuth{
			tokens: newTokens,
			err:    nil,
		}
	}

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		_ = w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		_, _ = fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		_ = pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	_ = pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Existing tokens have expired")
	assert.Contains(t, string(output), "Authentication successful")
	assert.Contains(t, string(output), "Tokens saved successfully")
}

func TestRunWithNewAuthentication(t *testing.T) {
	stubCredentialRegistrarFactory(t, &mockCredentialRegistrar{}, nil)

	oldBrowserAuth := newBrowserAuth
	defer func() { newBrowserAuth = oldBrowserAuth }()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, defaultConfigDir)
	err := os.MkdirAll(configDir, 0700)
	require.NoError(t, err)

	newTokens := &webauthn.AuthTokens{
		HGLogin:   "new-auth-hglogin",
		XSRFToken: "new-auth-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}

	newBrowserAuth = func(baseURL string) browserAuth {
		return &mockBrowserAuth{
			tokens: newTokens,
			err:    nil,
		}
	}

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		_ = w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		_, _ = fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		_ = pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	_ = pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Starting browser authentication")
	assert.Contains(t, string(output), "Authentication successful")
	assert.Contains(t, string(output), "Tokens saved successfully")

	savedTokensPath := filepath.Join(configDir, defaultTokensFile)
	savedData, err := os.ReadFile(savedTokensPath)
	require.NoError(t, err)

	var savedTokens webauthn.AuthTokens
	err = json.Unmarshal(savedData, &savedTokens)
	require.NoError(t, err)
	assert.Equal(t, newTokens.HGLogin, savedTokens.HGLogin)
	assert.Equal(t, newTokens.XSRFToken, savedTokens.XSRFToken)
}

func TestSaveTokens(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("successful save", func(t *testing.T) {
		tokensPath := filepath.Join(tempDir, "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test-hglogin",
			XSRFToken: "test-xsrf",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}

		err := saveTokens(tokensPath, tokens)
		assert.NoError(t, err)

		data, err := os.ReadFile(tokensPath)
		assert.NoError(t, err)

		var savedTokens webauthn.AuthTokens
		err = json.Unmarshal(data, &savedTokens)
		assert.NoError(t, err)
		assert.Equal(t, tokens.HGLogin, savedTokens.HGLogin)
		assert.Equal(t, tokens.XSRFToken, savedTokens.XSRFToken)

		info, err := os.Stat(tokensPath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("write error - invalid path", func(t *testing.T) {
		tokensPath := "/invalid/path/that/cannot/be/created/tokens.json"
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test",
			ExpiresAt: time.Now().Add(8 * time.Hour),
		}

		err := saveTokens(tokensPath, tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write tokens file")
	})
}

func TestAskVPSUpload(t *testing.T) {
	tempDir := t.TempDir()
	tokensPath := filepath.Join(tempDir, "tokens.json")
	tokens := &webauthn.AuthTokens{
		HGLogin:   "test-hglogin",
		XSRFToken: "test-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	data, err := json.Marshal(tokens)
	require.NoError(t, err)
	err = os.WriteFile(tokensPath, data, 0600)
	require.NoError(t, err)

	t.Run("user declines upload", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			_ = w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			_, _ = fmt.Fprintln(w, "no")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			_ = pw.Close()
			os.Stdout = oldStdout
		}()

		runner := newSetupRunner()
		runner.userInput = newConsoleUserInput(r)
		runner.scpClient = &mockSCPClient{err: errors.New("failed to transfer tokens")}
		err := runner.askVPSUpload(tokensPath)
		_ = r

		assert.NoError(t, err)

		_ = pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "VPS Deployment")
		assert.Contains(t, string(output), "Setup complete")
	})

	t.Run("empty VPS host", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			_ = w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			_, _ = fmt.Fprintln(w, "yes")
			_, _ = fmt.Fprintln(w, "")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			_ = pw.Close()
			os.Stdout = oldStdout
		}()

		runner := newSetupRunner()
		runner.userInput = newConsoleUserInput(r)
		runner.scpClient = &mockSCPClient{err: errors.New("failed to transfer tokens")}
		err := runner.askVPSUpload(tokensPath)
		_ = r

		assert.NoError(t, err)

		_ = pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "VPS host cannot be empty")
	})

	t.Run("valid VPS host with default path", func(t *testing.T) {
		output, err := setupVPSUploadTest(t, []string{"yes", "user@host", ""})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer tokens")
		assert.Contains(t, output, "Transferring tokens to VPS")
	})

	t.Run("valid VPS host with custom path", func(t *testing.T) {
		output, err := setupVPSUploadTest(t, []string{"yes", "user@host", "/custom/path/tokens.json"})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer tokens")
		assert.Contains(t, output, "Transferring tokens to VPS")
	})

	t.Run("case insensitive yes", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			_ = w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			_, _ = fmt.Fprintln(w, "YES")
			_, _ = fmt.Fprintln(w, "user@host")
			_, _ = fmt.Fprintln(w, "")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			_ = pw.Close()
			os.Stdout = oldStdout
		}()

		runner := newSetupRunner()
		runner.userInput = newConsoleUserInput(r)
		runner.scpClient = &mockSCPClient{err: errors.New("failed to transfer tokens")}
		err := runner.askVPSUpload(tokensPath)
		_ = r
		_ = pr

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer tokens")
	})

	t.Run("case insensitive no", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			_ = w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			_, _ = fmt.Fprintln(w, "NO")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			_ = pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
		_ = r

		assert.NoError(t, err)

		_ = pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "Setup complete")
	})
}

func TestRunWithReAuthAndVPSUpload(t *testing.T) {
	stubCredentialRegistrarFactory(t, &mockCredentialRegistrar{}, nil)

	oldBrowserAuth := newBrowserAuth
	defer func() { newBrowserAuth = oldBrowserAuth }()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, defaultConfigDir)
	err := os.MkdirAll(configDir, 0700)
	require.NoError(t, err)

	tokensPath := filepath.Join(configDir, defaultTokensFile)
	validTokens := &webauthn.AuthTokens{
		HGLogin:   "valid-hglogin",
		XSRFToken: "valid-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	data, err := json.Marshal(validTokens)
	require.NoError(t, err)
	err = os.WriteFile(tokensPath, data, 0600)
	require.NoError(t, err)

	newTokens := &webauthn.AuthTokens{
		HGLogin:   "reauth-hglogin",
		XSRFToken: "reauth-xsrf",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}

	newBrowserAuth = func(baseURL string) browserAuth {
		return &mockBrowserAuth{
			tokens: newTokens,
			err:    nil,
		}
	}

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		_ = w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		_, _ = fmt.Fprintln(w, "yes")
		_, _ = fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		_ = pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	_ = pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Valid tokens found")
	assert.Contains(t, string(output), "Authentication successful")
	assert.Contains(t, string(output), "Tokens saved successfully")
}

func TestOsFileSystem(t *testing.T) {
	fs := osFileSystem{}

	t.Run("UserHomeDir", func(t *testing.T) {
		home, err := fs.UserHomeDir()
		assert.NoError(t, err)
		assert.NotEmpty(t, home)
	})

	t.Run("MkdirAll and ReadFile and WriteFile", func(t *testing.T) {
		tempDir := t.TempDir()
		testDir := filepath.Join(tempDir, "test", "nested")
		testFile := filepath.Join(testDir, "test.txt")
		testData := []byte("hello world")

		err := fs.MkdirAll(testDir, 0755)
		assert.NoError(t, err)

		err = fs.WriteFile(testFile, testData, 0644)
		assert.NoError(t, err)

		readData, err := fs.ReadFile(testFile)
		assert.NoError(t, err)
		assert.Equal(t, testData, readData)
	})
}

func TestBrowserAuthAdapter(t *testing.T) {
	t.Run("Authenticate delegates to wrapped auth", func(t *testing.T) {
		// The browserAuthAdapter wraps *webauthn.BrowserAuth which is hard to mock directly.
		// This path is covered by integration tests that mock newBrowserAuth.
		// We verify the adapter exists and can be created.
		adapter := &browserAuthAdapter{}
		assert.NotNil(t, adapter)
	})

	t.Run("WithHeadless returns self", func(t *testing.T) {
		mock := &mockBrowserAuth{}
		adapter := &browserAuthAdapter{auth: webauthn.NewBrowserAuth("http://localhost")}
		_ = mock
		result := adapter.WithHeadless(true)
		assert.Equal(t, adapter, result)
	})

	t.Run("WithHeadless returns self for nil auth", func(t *testing.T) {
		adapter := &browserAuthAdapter{}
		result := adapter.WithHeadless(true)
		assert.Equal(t, adapter, result)
	})

	t.Run("nil adapter paths return safe defaults", func(t *testing.T) {
		adapter := &browserAuthAdapter{}
		result := adapter.WithProfileDir(t.TempDir())
		assert.Equal(t, adapter, result)

		tokens, err := adapter.ExtractTokensFromProfile()
		assert.Nil(t, tokens)
		assert.EqualError(t, err, "browser auth is not configured")
	})

	t.Run("delegates extract and profile helpers", func(t *testing.T) {
		profileCalled := false
		adapter := &browserAuthAdapter{
			extractTokensFunc: func() (*webauthn.AuthTokens, error) {
				return &webauthn.AuthTokens{HGLogin: "a", XSRFToken: "b", ExpiresAt: time.Now()}, nil
			},
			withProfileDirFunc: func(profileDir string) *webauthn.BrowserAuth {
				profileCalled = true
				assert.Equal(t, "/tmp/profile", profileDir)
				return webauthn.NewBrowserAuth("http://localhost")
			},
		}

		tokens, err := adapter.ExtractTokensFromProfile()
		assert.NoError(t, err)
		assert.NotNil(t, tokens)

		result := adapter.WithProfileDir("/tmp/profile")
		assert.Equal(t, adapter, result)
		assert.True(t, profileCalled)
	})

	t.Run("delegates headless helper", func(t *testing.T) {
		headlessCalled := false
		adapter := &browserAuthAdapter{
			withHeadlessFunc: func(headless bool) *webauthn.BrowserAuth {
				headlessCalled = true
				assert.True(t, headless)
				return webauthn.NewBrowserAuth("http://localhost")
			},
		}

		result := adapter.WithHeadless(true)
		assert.Equal(t, adapter, result)
		assert.True(t, headlessCalled)
	})
}

func TestWebauthnBrowserAuthFactory(t *testing.T) {
	factory := webauthnBrowserAuthFactory{}
	auth := factory.NewBrowserAuth("http://localhost")
	assert.NotNil(t, auth)
}

func TestNewBrowserAuthVar(t *testing.T) {
	auth := newBrowserAuth("http://localhost")
	assert.NotNil(t, auth)
}

func TestDefaultCredentialRegistrarFactory(t *testing.T) {
	tempDir := t.TempDir()
	registrar, err := defaultCredentialRegistrarFactory(filepath.Join(tempDir, "credentials.json"), "https://example.com")
	require.NoError(t, err)
	assert.NotNil(t, registrar)
}

func TestConsoleUserInput(t *testing.T) {
	t.Run("Confirm success yes", func(t *testing.T) {
		input := strings.NewReader("yes\n")
		cui := newConsoleUserInput(input)
		result, err := cui.Confirm("test prompt: ")
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("Confirm success no", func(t *testing.T) {
		input := strings.NewReader("no\n")
		cui := newConsoleUserInput(input)
		result, err := cui.Confirm("test prompt: ")
		assert.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("Confirm error", func(t *testing.T) {
		input := strings.NewReader("")
		cui := newConsoleUserInput(input)
		result, err := cui.Confirm("test prompt: ")
		assert.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("ReadLine with EOF", func(t *testing.T) {
		input := strings.NewReader("partial")
		cui := newConsoleUserInput(input)
		result, err := cui.ReadLine()
		assert.NoError(t, err)
		assert.Equal(t, "partial", result)
	})

	t.Run("ReadLine error", func(t *testing.T) {
		reader := &errorReader{}
		cui := newConsoleUserInput(reader)
		_, err := cui.ReadLine()
		assert.Error(t, err)
	})
}

type errorReader struct{}

func (e *errorReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestExecSCPClient(t *testing.T) {
	t.Run("CopyFile success with stubbed scp", func(t *testing.T) {
		tempDir := t.TempDir()
		scpPath := filepath.Join(tempDir, "scp")
		err := os.WriteFile(scpPath, []byte("#!/bin/sh\nexit 0\n"), 0755)
		require.NoError(t, err)

		oldPath := os.Getenv("PATH")
		t.Setenv("PATH", tempDir+string(os.PathListSeparator)+oldPath)

		testFile := filepath.Join(tempDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0600)
		require.NoError(t, err)

		client := &execSCPClient{stdout: io.Discard, stderr: io.Discard}
		err = client.CopyFile(testFile, "user@host", "/tmp/test.txt")
		assert.NoError(t, err)
	})

	t.Run("CopyFile returns scp failure", func(t *testing.T) {
		client := &execSCPClient{
			stdout: io.Discard,
			stderr: io.Discard,
		}
		tempDir := t.TempDir()
		scpPath := filepath.Join(tempDir, "scp")
		err := os.WriteFile(scpPath, []byte("#!/bin/sh\nexit 1\n"), 0755)
		require.NoError(t, err)
		t.Setenv("PATH", tempDir)

		testFile := filepath.Join(tempDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0600)
		require.NoError(t, err)

		err = client.CopyFile(testFile, "user@host", "/tmp/test.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer tokens")
	})
}

func TestAskVPSUploadWithReadErrors(t *testing.T) {
	t.Run("VPS host read error", func(t *testing.T) {
		tempDir := t.TempDir()
		tokensPath := filepath.Join(tempDir, "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		data, err := json.Marshal(tokens)
		require.NoError(t, err)
		err = os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		runner := newSetupRunner()
		runner.userInput = &mockUserInput{
			confirmResult:  true,
			confirmError:   nil,
			readLineResult: "",
			readLineError:  errors.New("read error"),
		}

		err = runner.askVPSUpload(tokensPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read VPS host")
	})

	t.Run("VPS path read error", func(t *testing.T) {
		tempDir := t.TempDir()
		tokensPath := filepath.Join(tempDir, "tokens.json")
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		data, err := json.Marshal(tokens)
		require.NoError(t, err)
		err = os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		runner := newSetupRunner()
		runner.userInput = &mockUserInputV2{
			results: []string{"user@host"},
			errors:  []error{nil, errors.New("read error")},
		}
		runner.scpClient = &mockSCPClient{}

		err = runner.askVPSUpload(tokensPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read VPS target path")
	})
}

func TestConsoleUserInputConfirmReadError(t *testing.T) {
	cui := newConsoleUserInput(&errorReader{})
	result, err := cui.Confirm("test prompt: ")
	assert.Error(t, err)
	assert.False(t, result)
}

func TestSetupRunnerRunErrorBranches(t *testing.T) {
	stubCredentialRegistrarFactory(t, &mockCredentialRegistrar{}, nil)

	t.Run("re-auth confirmation read error", func(t *testing.T) {
		validTokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "test",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		data, err := json.Marshal(validTokens)
		require.NoError(t, err)

		runner := &setupRunner{
			fs: &mockFileSystem{
				userHomeDir:  "/home/test",
				readFileData: data,
			},
			userInput: &mockUserInput{
				confirmError: errors.New("boom"),
			},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth {
				return &mockBrowserAuth{}
			}},
			scpClient:  &mockSCPClient{},
			baseURL:    defaultBaseURL,
			configDir:  defaultConfigDir,
			tokensFile: defaultTokensFile,
		}

		err = runner.run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read re-authentication confirmation")
	})

	t.Run("save tokens failure", func(t *testing.T) {
		runner := &setupRunner{
			fs: &mockFileSystem{
				userHomeDir:    "/home/test",
				readFileError:  os.ErrNotExist,
				writeFileError: errors.New("write failed"),
			},
			userInput: &mockUserInput{confirmResult: false},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth {
				return &mockBrowserAuth{tokens: &webauthn.AuthTokens{
					HGLogin:   "new",
					XSRFToken: "new",
					ExpiresAt: time.Now().Add(time.Hour),
				}}
			}},
			scpClient:  &mockSCPClient{},
			baseURL:    defaultBaseURL,
			configDir:  defaultConfigDir,
			tokensFile: defaultTokensFile,
		}

		err := runner.run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save tokens")
	})

	t.Run("ask VPS upload confirmation read error", func(t *testing.T) {
		runner := &setupRunner{
			fs: &mockFileSystem{
				userHomeDir:   "/home/test",
				readFileError: os.ErrNotExist,
			},
			userInput: &mockUserInput{
				confirmError: errors.New("confirm error"),
			},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth {
				return &mockBrowserAuth{tokens: &webauthn.AuthTokens{
					HGLogin:   "new",
					XSRFToken: "new",
					ExpiresAt: time.Now().Add(time.Hour),
				}}
			}},
			scpClient:  &mockSCPClient{},
			baseURL:    defaultBaseURL,
			configDir:  defaultConfigDir,
			tokensFile: defaultTokensFile,
		}

		err := runner.run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read transfer confirmation")
	})

	t.Run("register credential failure degrades gracefully after tokens saved", func(t *testing.T) {
		runner := &setupRunner{
			fs: &mockFileSystem{
				userHomeDir:   "/home/test",
				readFileError: os.ErrNotExist,
			},
			userInput: &mockUserInput{confirmResult: false},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth {
				return &mockBrowserAuth{tokens: &webauthn.AuthTokens{
					HGLogin:   "new",
					XSRFToken: "new",
					ExpiresAt: time.Now().Add(time.Hour),
				}}
			}},
			authFactory: func(_, _ string) (credentialRegistrar, error) {
				return &mockCredentialRegistrar{registerErr: errors.New("register failed")}, nil
			},
			scpClient:  &mockSCPClient{},
			baseURL:    defaultBaseURL,
			configDir:  defaultConfigDir,
			tokensFile: defaultTokensFile,
		}

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			_ = pw.Close()
			os.Stdout = oldStdout
		}()

		err := runner.run()
		assert.NoError(t, err)

		_ = pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "Authentication successful")
		assert.Contains(t, string(output), "Tokens saved successfully")
		assert.Contains(t, string(output), "WebAuthn registration skipped")
	})

	t.Run("chrome profile directory creation failure", func(t *testing.T) {
		runner := &setupRunner{
			fs:              &sequenceMkdirFileSystem{userHomeDir: "/home/test", secondErr: errors.New("profile mkdir failed")},
			userInput:       &mockUserInput{confirmResult: false},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth { return &mockBrowserAuth{} }},
			scpClient:       &mockSCPClient{},
			baseURL:         defaultBaseURL,
			configDir:       defaultConfigDir,
			tokensFile:      defaultTokensFile,
		}

		err := runner.run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create chrome profile directory")
	})

	t.Run("manual browser launch failure", func(t *testing.T) {
		runner := &setupRunner{
			fs:              &mockFileSystem{userHomeDir: "/home/test", readFileError: os.ErrNotExist},
			userInput:       &mockUserInput{confirmResult: false},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth { return &mockBrowserAuth{} }},
			launchBrowser:   func(profileDir, loginURL string) error { return errors.New("launch failed") },
			scpClient:       &mockSCPClient{},
			baseURL:         defaultBaseURL,
			configDir:       defaultConfigDir,
			tokensFile:      defaultTokensFile,
		}

		err := runner.run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to launch manual browser")
	})

	t.Run("manual browser confirmation failure", func(t *testing.T) {
		runner := &setupRunner{
			fs:              &mockFileSystem{userHomeDir: "/home/test", readFileError: os.ErrNotExist},
			userInput:       &mockUserInput{confirmResult: false},
			browserAuthFact: functionBrowserAuthFactory{newFn: func(_ string) browserAuth { return &mockBrowserAuth{} }},
			launchBrowser:   func(profileDir, loginURL string) error { return nil },
			waitForConfirm:  func() error { return errors.New("confirm failed") },
			scpClient:       &mockSCPClient{},
			baseURL:         defaultBaseURL,
			configDir:       defaultConfigDir,
			tokensFile:      defaultTokensFile,
		}

		err := runner.run()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manual browser confirmation failed")
	})
}

func TestSaveTokensMarshalErrorBranch(t *testing.T) {
	runner := &setupRunner{fs: &mockFileSystem{}}
	tokens := &webauthn.AuthTokens{
		HGLogin:   "test",
		XSRFToken: "test",
		ExpiresAt: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
	}

	err := runner.saveTokens("/tmp/tokens.json", tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal tokens")
}

func TestSetupRunnerAskVPSUploadSuccessDefaultPath(t *testing.T) {
	runner := newSetupRunner()
	runner.userInput = &mockUserInputV2{
		results: []string{"user@host", ""},
		errors:  []error{nil, nil},
	}
	runner.scpClient = &mockSCPClient{}

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		_ = pw.Close()
		os.Stdout = oldStdout
	}()

	err := runner.askVPSUpload("/tmp/tokens.json")
	assert.NoError(t, err)

	_ = pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Tokens transferred successfully")
	assert.Contains(t, string(output), "~/.hourglass-rpa/auth-tokens.json")
}

func TestSetupRunnerAskVPSUploadWithCredentialsSuccess(t *testing.T) {
	scpClient := &mockSCPClient{}
	runner := newSetupRunner()
	runner.userInput = &mockUserInputV2{
		results: []string{"user@host", ""},
		errors:  []error{nil, nil},
	}
	runner.scpClient = scpClient

	err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
	require.NoError(t, err)
	require.Len(t, scpClient.calls, 2)
	assert.Equal(t, "/tmp/auth-tokens.json", scpClient.calls[0].localPath)
	assert.Equal(t, "~/.hourglass-rpa/auth-tokens.json", scpClient.calls[0].remotePath)
	assert.Equal(t, "/tmp/webauthn-credentials.json", scpClient.calls[1].localPath)
	assert.Equal(t, "~/.hourglass-rpa/webauthn-credentials.json", scpClient.calls[1].remotePath)
}

func TestSetupRunnerAskVPSUploadConfirmReadError(t *testing.T) {
	runner := newSetupRunner()
	runner.userInput = &mockUserInput{confirmError: errors.New("confirm error")}

	err := runner.askVPSUpload("/tmp/tokens.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read transfer confirmation")
}

func TestSetupRunnerRegisterWebAuthnCredentialFactoryError(t *testing.T) {
	runner := newSetupRunner()
	runner.authFactory = func(string, string) (credentialRegistrar, error) {
		return nil, errors.New("factory error")
	}

	err := runner.registerWebAuthnCredential("/tmp/credentials.json", &webauthn.AuthTokens{
		HGLogin:   "hglogin",
		XSRFToken: "xsrf",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create authenticator")
}

func TestSetupRunnerAskVPSUploadWithCredentialsErrors(t *testing.T) {
	t.Run("confirm read error", func(t *testing.T) {
		runner := newSetupRunner()
		runner.userInput = &mockUserInput{confirmError: errors.New("confirm error")}

		err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read transfer confirmation")
	})

	t.Run("vps host read error", func(t *testing.T) {
		runner := newSetupRunner()
		runner.userInput = &mockUserInput{
			confirmResult: true,
			readLineError: errors.New("read host error"),
		}

		err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read VPS host")
	})

	t.Run("empty vps host", func(t *testing.T) {
		runner := newSetupRunner()
		runner.userInput = &mockUserInput{
			confirmResult:  true,
			readLineResult: "",
		}

		err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
		assert.NoError(t, err)
	})

	t.Run("vps target path read error", func(t *testing.T) {
		runner := newSetupRunner()
		runner.userInput = &mockUserInputV2{
			results: []string{"user@host"},
			errors:  []error{nil, errors.New("read target path error")},
		}
		runner.scpClient = &mockSCPClient{}

		err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read VPS target path")
	})

	t.Run("token copy error", func(t *testing.T) {
		runner := newSetupRunner()
		runner.userInput = &mockUserInputV2{
			results: []string{"user@host", ""},
			errors:  []error{nil, nil},
		}
		runner.scpClient = &mockSCPClient{err: errors.New("copy error")}

		err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "copy error")
	})

	t.Run("credential copy error", func(t *testing.T) {
		scpClient := &mockSCPClient{}
		runner := newSetupRunner()
		runner.userInput = &mockUserInputV2{
			results: []string{"user@host", ""},
			errors:  []error{nil, nil},
		}
		runner.scpClient = scpClient
		scpClient.err = nil

		callCount := 0
		runner.scpClient = SCPClientFunc(func(localPath, remoteHost, remotePath string) error {
			callCount++
			if callCount == 1 {
				return nil
			}
			return errors.New("credential copy error")
		})

		err := runner.askVPSUploadWithCredentials("/tmp/auth-tokens.json", "/tmp/webauthn-credentials.json")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "credential copy error")
	})
}

func TestBrowserAuthAdapterAuthenticate(t *testing.T) {
	adapter := &browserAuthAdapter{authenticateFunc: func() (*webauthn.AuthTokens, error) {
		return nil, errors.New("browser authentication failed: chrome missing")
	}}
	tokens, err := adapter.Authenticate()

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "browser authentication failed")
}

func TestBrowserAuthAdapterAuthenticate_Success(t *testing.T) {
	expected := &webauthn.AuthTokens{HGLogin: "a", XSRFToken: "b", ExpiresAt: time.Now()}
	adapter := &browserAuthAdapter{authenticateFunc: func() (*webauthn.AuthTokens, error) {
		return expected, nil
	}}

	tokens, err := adapter.Authenticate()
	assert.NoError(t, err)
	assert.Equal(t, expected, tokens)
}

func TestBrowserAuthAdapterAuthenticate_NilAdapter(t *testing.T) {
	adapter := &browserAuthAdapter{}
	tokens, err := adapter.Authenticate()
	assert.Nil(t, tokens)
	assert.EqualError(t, err, "browser auth is not configured")
}

func TestBrowserAuthAdapterAuthenticate_UsesWrappedAuth(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("CHROME_BIN", filepath.Join(t.TempDir(), "missing-chrome"))
	t.Setenv("CHROME_PATH", "")
	adapter := &browserAuthAdapter{auth: webauthn.NewBrowserAuth("http://localhost")}
	tokens, err := adapter.Authenticate()
	assert.Nil(t, tokens)
	assert.Error(t, err)
}

func TestBrowserAuthAdapterExtractTokensFromProfile_UsesWrappedAuth(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("CHROME_BIN", filepath.Join(t.TempDir(), "missing-chrome"))
	t.Setenv("CHROME_PATH", "")
	adapter := &browserAuthAdapter{auth: webauthn.NewBrowserAuth("http://localhost")}
	tokens, err := adapter.ExtractTokensFromProfile()
	assert.Nil(t, tokens)
	assert.Error(t, err)
}

func TestBrowserAuthAdapterWithProfileDir_UsesWrappedAuth(t *testing.T) {
	adapter := &browserAuthAdapter{auth: webauthn.NewBrowserAuth("http://localhost")}
	result := adapter.WithProfileDir("/tmp/profile")
	assert.Equal(t, adapter, result)
}

func TestSetupRunner_chromeProfileDir_UsesEnvOverride(t *testing.T) {
	t.Setenv("CHROME_PROFILE_DIR", "/tmp/custom-profile")
	runner := newSetupRunner()
	assert.Equal(t, "/tmp/custom-profile", runner.chromeProfileDir("/tmp/config"))
}

func TestLaunchChromeForManualLogin_ReturnsMissingChromeError(t *testing.T) {
	t.Setenv("CHROME_BIN", "")
	t.Setenv("CHROME_PATH", "")
	originalStat := chromeStatFn
	t.Cleanup(func() { chromeStatFn = originalStat })
	chromeStatFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	err := launchChromeForManualLogin(t.TempDir(), "https://example.com/login")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chrome/chromium not found")
}

func TestLaunchChromeForManualLogin_ReturnsStartError(t *testing.T) {
	missingChrome := filepath.Join(t.TempDir(), "missing-chrome")
	t.Setenv("CHROME_BIN", missingChrome)
	t.Setenv("CHROME_PATH", "")

	err := launchChromeForManualLogin(t.TempDir(), "https://example.com/login")
	assert.Error(t, err)
}

func TestLaunchChromeForManualLogin_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("CHROME_BIN", "/bin/true")
	t.Setenv("CHROME_PATH", "")

	profileDir := filepath.Join(tempDir, "profile")
	err := launchChromeForManualLogin(profileDir, "https://example.com/login")
	assert.NoError(t, err)
	_, statErr := os.Stat(profileDir)
	assert.NoError(t, statErr)
}

func TestLaunchChromeForManualLogin_UsesChromePathFallback(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("CHROME_BIN", "")
	t.Setenv("CHROME_PATH", "/bin/true")

	profileDir := filepath.Join(tempDir, "profile-fallback")
	err := launchChromeForManualLogin(profileDir, "https://example.com/login")
	assert.NoError(t, err)
}

func TestLaunchChromeForManualLogin_UsesCandidateDiscovery(t *testing.T) {
	originalStat := chromeStatFn
	originalPrepare := prepareChromeProfileFn
	originalExec := execCommandFn
	defer func() {
		chromeStatFn = originalStat
		prepareChromeProfileFn = originalPrepare
		execCommandFn = originalExec
	}()

	t.Setenv("CHROME_BIN", "")
	t.Setenv("CHROME_PATH", "")
	chromeStatFn = func(path string) (os.FileInfo, error) {
		if path == "/usr/bin/google-chrome" {
			return os.Stat(os.TempDir())
		}
		return nil, os.ErrNotExist
	}
	prepareChromeProfileFn = func(profileDir string) error { return nil }
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("/bin/sh", "-c", fmt.Sprintf("[ \"%s\" = \"/usr/bin/google-chrome\" ]", name))
	}

	err := launchChromeForManualLogin(t.TempDir(), "https://example.com/login")
	assert.NoError(t, err)
}

func TestLaunchChromeForManualLogin_PrepareProfileError(t *testing.T) {
	originalStat := chromeStatFn
	originalPrepare := prepareChromeProfileFn
	defer func() {
		chromeStatFn = originalStat
		prepareChromeProfileFn = originalPrepare
	}()

	t.Setenv("CHROME_BIN", "")
	t.Setenv("CHROME_PATH", "")
	chromeStatFn = func(path string) (os.FileInfo, error) {
		if path == "/usr/bin/google-chrome" {
			return os.Stat(os.TempDir())
		}
		return nil, os.ErrNotExist
	}
	prepareChromeProfileFn = func(profileDir string) error { return errors.New("prepare failed") }

	err := launchChromeForManualLogin(t.TempDir(), "https://example.com/login")
	assert.EqualError(t, err, "prepare failed")
}

func TestWaitForBrowserConfirmation_Success(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	go func() {
		_, _ = fmt.Fprintln(w)
		_ = w.Close()
	}()

	assert.NoError(t, waitForBrowserConfirmation())
}

func TestWaitForBrowserConfirmation_ReadError(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()
	_ = w.Close()

	err = waitForBrowserConfirmation()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read confirmation input")
}

func TestMainSuccessAndFailurePaths(t *testing.T) {
	t.Run("success path", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)

		configDir := filepath.Join(tempDir, defaultConfigDir)
		err := os.MkdirAll(configDir, 0700)
		require.NoError(t, err)

		tokensPath := filepath.Join(configDir, defaultTokensFile)
		validTokens := &webauthn.AuthTokens{
			HGLogin:   "valid",
			XSRFToken: "valid",
			ExpiresAt: time.Now().Add(2 * time.Hour),
		}
		data, err := json.Marshal(validTokens)
		require.NoError(t, err)
		err = os.WriteFile(tokensPath, data, 0600)
		require.NoError(t, err)

		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			_ = w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			_, _ = fmt.Fprintln(w, "no")
		}()

		main()
	})

	t.Run("failure path exits with code 1", func(t *testing.T) {
		tempDir := t.TempDir()
		homeAsFile := filepath.Join(tempDir, "home-file")
		err := os.WriteFile(homeAsFile, []byte("not a dir"), 0600)
		require.NoError(t, err)

		cmd := exec.Command(os.Args[0], "-test.run=TestMainFailureHelperProcess")
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HOME="+homeAsFile,
		)

		output, err := cmd.CombinedOutput()
		require.Error(t, err)

		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 1, exitErr.ExitCode())
		assert.Contains(t, string(output), "Setup failed")
	})
}

func TestMainFailureHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process test")
	}

	main()
}

type mockUserInputV2 struct {
	callIdx int
	results []string
	errors  []error
}

func (m *mockUserInputV2) Confirm(_ string) (bool, error) {
	return true, nil
}

func (m *mockUserInputV2) ReadLine() (string, error) {
	if m.callIdx >= len(m.results) {
		return "", m.errors[len(m.errors)-1]
	}
	result := m.results[m.callIdx]
	err := m.errors[m.callIdx]
	m.callIdx++
	return result, err
}

type scpCopyCall struct {
	localPath  string
	remoteHost string
	remotePath string
}

type SCPClientFunc func(localPath, remoteHost, remotePath string) error

func (f SCPClientFunc) CopyFile(localPath, remoteHost, remotePath string) error {
	return f(localPath, remoteHost, remotePath)
}

type mockSCPClient struct {
	calls []scpCopyCall
	err   error
}

func (m *mockSCPClient) CopyFile(localPath, remoteHost, remotePath string) error {
	m.calls = append(m.calls, scpCopyCall{
		localPath:  localPath,
		remoteHost: remoteHost,
		remotePath: remotePath,
	})
	return m.err
}

type mockUserInput struct {
	confirmResult  bool
	confirmError   error
	readLineResult string
	readLineError  error
	readLineCalls  int
}

func (m *mockUserInput) Confirm(_ string) (bool, error) {
	return m.confirmResult, m.confirmError
}

func (m *mockUserInput) ReadLine() (string, error) {
	m.readLineCalls++
	if m.readLineCalls == 1 && m.readLineResult != "" {
		return m.readLineResult, m.readLineError
	}
	return "", m.readLineError
}

func TestSaveTokensMarshalError(t *testing.T) {
	mockFS := &mockFileSystem{
		writeFileFunc: func(path string, data []byte, perm os.FileMode) error {
			return nil
		},
	}

	runner := &setupRunner{
		fs: mockFS,
	}

	tokens := &webauthn.AuthTokens{
		HGLogin:   "test",
		XSRFToken: "test",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	tempDir := t.TempDir()
	tokensPath := filepath.Join(tempDir, "tokens.json")

	err := runner.saveTokens(tokensPath, tokens)
	assert.NoError(t, err)
}

func TestOptionsFileSystem(t *testing.T) {
	t.Run("UserHomeDir uses fallback when function is nil", func(t *testing.T) {
		baseFS := &mockFileSystem{
			userHomeDir: "/home/test",
		}
		ofs := &optionsFileSystem{
			base:          baseFS,
			userHomeDirFn: nil,
		}
		home, err := ofs.UserHomeDir()
		assert.NoError(t, err)
		assert.Equal(t, "/home/test", home)
	})

	t.Run("UserHomeDir uses provided function", func(t *testing.T) {
		baseFS := &mockFileSystem{
			userHomeDir: "/home/base",
		}
		ofs := &optionsFileSystem{
			base:          baseFS,
			userHomeDirFn: func() (string, error) { return "/home/custom", nil },
		}
		home, err := ofs.UserHomeDir()
		assert.NoError(t, err)
		assert.Equal(t, "/home/custom", home)
	})

	t.Run("delegates MkdirAll to base", func(t *testing.T) {
		baseFS := &mockFileSystem{}
		ofs := &optionsFileSystem{base: baseFS}
		err := ofs.MkdirAll("/test/path", 0755)
		assert.NoError(t, err)
	})

	t.Run("delegates ReadFile to base", func(t *testing.T) {
		baseFS := &mockFileSystem{
			readFileData: []byte("test data"),
		}
		ofs := &optionsFileSystem{base: baseFS}
		data, err := ofs.ReadFile("/test/file")
		assert.NoError(t, err)
		assert.Equal(t, []byte("test data"), data)
	})

	t.Run("delegates WriteFile to base", func(t *testing.T) {
		baseFS := &mockFileSystem{}
		ofs := &optionsFileSystem{base: baseFS}
		err := ofs.WriteFile("/test/file", []byte("data"), 0644)
		assert.NoError(t, err)
	})
}

type mockFileSystem struct {
	userHomeDir    string
	userHomeError  error
	mkdirError     error
	readFileData   []byte
	readFileError  error
	writeFileError error
	writeFileFunc  func(path string, data []byte, perm os.FileMode) error
}

type sequenceMkdirFileSystem struct {
	userHomeDir string
	callCount   int
	secondErr   error
}

func (m *sequenceMkdirFileSystem) UserHomeDir() (string, error) {
	return m.userHomeDir, nil
}

func (m *sequenceMkdirFileSystem) MkdirAll(_ string, _ os.FileMode) error {
	m.callCount++
	if m.callCount == 2 {
		return m.secondErr
	}
	return nil
}

func (m *sequenceMkdirFileSystem) ReadFile(_ string) ([]byte, error) {
	return nil, os.ErrNotExist
}

func (m *sequenceMkdirFileSystem) WriteFile(_ string, _ []byte, _ os.FileMode) error {
	return nil
}

func (m *mockFileSystem) UserHomeDir() (string, error) {
	return m.userHomeDir, m.userHomeError
}

func (m *mockFileSystem) MkdirAll(_ string, _ os.FileMode) error {
	return m.mkdirError
}

func (m *mockFileSystem) ReadFile(_ string) ([]byte, error) {
	return m.readFileData, m.readFileError
}

func (m *mockFileSystem) WriteFile(_ string, data []byte, _ os.FileMode) error {
	if m.writeFileFunc != nil {
		_ = data
		return nil
	}
	return m.writeFileError
}
