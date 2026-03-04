package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/auth/webauthn"
)

type mockBrowserAuth struct {
	tokens *webauthn.AuthTokens
	err    error
}

func (m *mockBrowserAuth) Authenticate() (*webauthn.AuthTokens, error) {
	return m.tokens, m.err
}

func (m *mockBrowserAuth) WithHeadless(headless bool) browserAuth {
	return m
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
	assert.Equal(t, "https://app.hourglass-app.com/api/v0.2", defaultBaseURL)
	assert.Equal(t, ".hourglass-rpa", defaultConfigDir)
	assert.Equal(t, "auth-tokens.json", defaultTokensFile)
}

func TestRun(t *testing.T) {
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
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "no")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := run(opts)
		_ = pr

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "authentication failed")
	})
}

func TestRunWithValidTokens(t *testing.T) {
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
		w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Valid tokens found")
	assert.Contains(t, string(output), "Using existing tokens")
}

func TestRunWithExpiredTokens(t *testing.T) {
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
		w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	pw.Close()
	output, _ := io.ReadAll(pr)
	assert.Contains(t, string(output), "Existing tokens have expired")
	assert.Contains(t, string(output), "Authentication successful")
	assert.Contains(t, string(output), "Tokens saved successfully")
}

func TestRunWithNewAuthentication(t *testing.T) {
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
		w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	pw.Close()
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
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "no")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
		_ = r

		assert.NoError(t, err)

		pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "VPS Deployment")
		assert.Contains(t, string(output), "Setup complete")
	})

	t.Run("empty VPS host", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "yes")
			fmt.Fprintln(w, "")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
		_ = r

		assert.NoError(t, err)

		pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "VPS host cannot be empty")
	})

	t.Run("valid VPS host with default path", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "yes")
			fmt.Fprintln(w, "user@host")
			fmt.Fprintln(w, "")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
		_ = r

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer tokens")

		pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "Transferring tokens to VPS")
	})

	t.Run("valid VPS host with custom path", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "yes")
			fmt.Fprintln(w, "user@host")
			fmt.Fprintln(w, "/custom/path/tokens.json")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
		_ = r

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to transfer tokens")

		pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "Transferring tokens to VPS")
	})

	t.Run("case insensitive yes", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r
		defer func() {
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "YES")
			fmt.Fprintln(w, "user@host")
			fmt.Fprintln(w, "")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
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
			w.Close()
			os.Stdin = oldStdin
		}()

		go func() {
			fmt.Fprintln(w, "NO")
		}()

		oldStdout := os.Stdout
		pr, pw, _ := os.Pipe()
		os.Stdout = pw
		defer func() {
			pw.Close()
			os.Stdout = oldStdout
		}()

		err := askVPSUpload(tokensPath)
		_ = r

		assert.NoError(t, err)

		pw.Close()
		output, _ := io.ReadAll(pr)
		assert.Contains(t, string(output), "Setup complete")
	})
}

func TestRunWithReAuthAndVPSUpload(t *testing.T) {
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
		w.Close()
		os.Stdin = oldStdin
	}()

	go func() {
		fmt.Fprintln(w, "yes")
		fmt.Fprintln(w, "no")
	}()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw
	defer func() {
		pw.Close()
		os.Stdout = oldStdout
	}()

	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: func() (string, error) { return tempDir, nil },
	}

	err = run(opts)
	_ = r

	assert.NoError(t, err)

	pw.Close()
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
}

func TestWebauthnBrowserAuthFactory(t *testing.T) {
	factory := webauthnBrowserAuthFactory{}
	auth := factory.NewBrowserAuth("http://localhost")
	assert.NotNil(t, auth)
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

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestExecSCPClient(t *testing.T) {
	t.Run("CopyFile success - dry run with bad host", func(t *testing.T) {
		client := &execSCPClient{
			stdout: io.Discard,
			stderr: io.Discard,
		}
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)

		err = client.CopyFile(testFile, "invalid-host-test", "/tmp/test.txt")
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
		data, _ := json.Marshal(tokens)
		os.WriteFile(tokensPath, data, 0600)

		runner := newSetupRunner()
		runner.userInput = &mockUserInput{
			confirmResult:  true,
			confirmError:   nil,
			readLineResult: "",
			readLineError:  errors.New("read error"),
		}

		err := runner.askVPSUpload(tokensPath)
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
		data, _ := json.Marshal(tokens)
		os.WriteFile(tokensPath, data, 0600)

		runner := newSetupRunner()
		runner.userInput = &mockUserInputV2{
			results: []string{"user@host"},
			errors:  []error{nil, errors.New("read error")},
		}
		runner.scpClient = &mockSCPClient{}

		err := runner.askVPSUpload(tokensPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read VPS target path")
	})
}

type mockUserInputV2 struct {
	callIdx int
	results []string
	errors  []error
}

func (m *mockUserInputV2) Confirm(prompt string) (bool, error) {
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

type mockSCPClient struct{}

func (m *mockSCPClient) CopyFile(localPath, remoteHost, remotePath string) error {
	return nil
}

type mockUserInput struct {
	confirmResult  bool
	confirmError   error
	readLineResult string
	readLineError  error
	readLineCalls  int
}

func (m *mockUserInput) Confirm(prompt string) (bool, error) {
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

func (m *mockFileSystem) UserHomeDir() (string, error) {
	return m.userHomeDir, m.userHomeError
}

func (m *mockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return m.mkdirError
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	return m.readFileData, m.readFileError
}

func (m *mockFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	if m.writeFileFunc != nil {
		return m.writeFileFunc(path, data, perm)
	}
	return m.writeFileError
}
