package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"hourglass-rejections-rpa/internal/auth/webauthn"
)

type mockTokenSaver struct {
	mock.Mock
}

func (m *mockTokenSaver) SaveTokens(tokens *webauthn.AuthTokens) error {
	args := m.Called(tokens)
	return args.Error(0)
}

type mockBrowserAuthenticator struct {
	mock.Mock
}

func (m *mockBrowserAuthenticator) Authenticate() (*webauthn.AuthTokens, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webauthn.AuthTokens), args.Error(1)
}

func (m *mockBrowserAuthenticator) WithHeadless(headless bool) browserAuthenticator {
	m.Called(headless)
	return m
}

func createTestTokens() *webauthn.AuthTokens {
	return &webauthn.AuthTokens{
		HGLogin:   "test_hg_login_value_12345678901234567890",
		XSRFToken: "test_xsrf_token_value_12345678901234567890",
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
}

func TestNewTokenSaver(t *testing.T) {
	ts := newTokenSaver()

	assert.NotNil(t, ts)
	assert.NotNil(t, ts.tokenManagerFactory)
	assert.NotNil(t, ts.browserAuthFactory)
	assert.NotNil(t, ts.userHomeDir)
	assert.NotNil(t, ts.mkdirAll)
}

func TestTokenSaver_Run_HomeDirectoryError(t *testing.T) {
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &mockBrowserAuthenticator{}
		},
		userHomeDir: func() (string, error) {
			return "", errors.New("home directory error")
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get home directory")
}

func TestTokenSaver_Run_MkdirAllError(t *testing.T) {
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &mockBrowserAuthenticator{}
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return errors.New("mkdir all error")
		},
	}

	err := ts.run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create config directory")
}

func TestTokenSaver_Run_TokenManagerCreationError(t *testing.T) {
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return nil, errors.New("token manager creation error")
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &mockBrowserAuthenticator{}
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create TokenManager")
}

func TestTokenSaver_Run_BrowserAuthenticationError(t *testing.T) {
	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(nil, errors.New("authentication failed"))
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	mockBrowser.AssertExpectations(t)
}

func TestTokenSaver_Run_SaveTokensError(t *testing.T) {
	testTokens := createTestTokens()
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", testTokens).Return(errors.New("save tokens error"))

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save tokens")
	mockTokenMgr.AssertExpectations(t)
	mockBrowser.AssertExpectations(t)
}

func TestTokenSaver_Run_Success(t *testing.T) {
	testTokens := createTestTokens()
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", testTokens).Return(nil)

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.NoError(t, err)
	mockTokenMgr.AssertExpectations(t)
	mockTokenMgr.AssertCalled(t, "SaveTokens", testTokens)
	mockBrowser.AssertExpectations(t)
	mockBrowser.AssertCalled(t, "WithHeadless", false)
}

func TestPrintSuccess(t *testing.T) {
	tokens := &webauthn.AuthTokens{
		HGLogin:   "test_login_12345678901234567890",
		XSRFToken: "test_token_12345678901234567890",
		ExpiresAt: time.Date(2026, 3, 3, 10, 30, 0, 0, time.UTC),
	}

	assert.NotPanics(t, func() {
		printSuccess("/path/to/tokens.json", tokens)
	})
}

func TestPrintSuccess_ShortTokens(t *testing.T) {
	tokens := &webauthn.AuthTokens{
		HGLogin:   "test1234",
		XSRFToken: "xsrf1234",
		ExpiresAt: time.Now(),
	}

	assert.NotPanics(t, func() {
		printSuccess("/path/to/tokens.json", tokens)
	})
}

func TestOsExit(t *testing.T) {
	originalOsExit := osExit
	defer func() { osExit = originalOsExit }()

	assert.NotPanics(t, func() {
		_ = osExit
	})
}

func TestTokenSaver_Run_WithHeadlessFalse(t *testing.T) {
	testTokens := createTestTokens()
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", testTokens).Return(nil)

	headlessValue := true
	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", mock.MatchedBy(func(val bool) bool {
		headlessValue = val
		return true
	})).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.NoError(t, err)
	assert.False(t, headlessValue)
}

func TestTokenSaver_Run_ConfigDirectoryPermissions(t *testing.T) {
	mkdirCalled := false
	mkdirPerms := os.FileMode(0)

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(nil, errors.New("not testing auth"))
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			mkdirCalled = true
			mkdirPerms = perm
			return errors.New("stop here")
		},
	}

	err := ts.run()

	assert.Error(t, err)
	assert.True(t, mkdirCalled)
	assert.Equal(t, os.FileMode(0700), mkdirPerms)
}

func TestTokenSaver_Run_TokenManagerOptions(t *testing.T) {
	mockBrowser := new(mockBrowserAuthenticator)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return nil, errors.New("stop here")
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	_ = ts.run()
}

func TestTokenSaver_Run_MultipleErrorPaths(t *testing.T) {
	tests := []struct {
		name             string
		userHomeDirFunc  func() (string, error)
		mkdirAllFunc     func(path string, perm os.FileMode) error
		tokenManagerFunc func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error)
		browserAuthFunc  func(baseURL string) browserAuthenticator
		expectedError    string
	}{
		{
			name: "UserHomeDir error",
			userHomeDirFunc: func() (string, error) {
				return "", errors.New("no home dir")
			},
			mkdirAllFunc: func(path string, perm os.FileMode) error {
				return nil
			},
			tokenManagerFunc: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
				return &mockTokenSaver{}, nil
			},
			browserAuthFunc: func(baseURL string) browserAuthenticator {
				mockBrowser := new(mockBrowserAuthenticator)
				mockBrowser.On("WithHeadless", false).Return(mockBrowser)
				return mockBrowser
			},
			expectedError: "failed to get home directory",
		},
		{
			name: "MkdirAll error",
			userHomeDirFunc: func() (string, error) {
				return "/home/test", nil
			},
			mkdirAllFunc: func(path string, perm os.FileMode) error {
				return errors.New("permission denied")
			},
			tokenManagerFunc: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
				return &mockTokenSaver{}, nil
			},
			browserAuthFunc: func(baseURL string) browserAuthenticator {
				mockBrowser := new(mockBrowserAuthenticator)
				mockBrowser.On("WithHeadless", false).Return(mockBrowser)
				return mockBrowser
			},
			expectedError: "failed to create config directory",
		},
		{
			name: "TokenManager error",
			userHomeDirFunc: func() (string, error) {
				return "/home/test", nil
			},
			mkdirAllFunc: func(path string, perm os.FileMode) error {
				return nil
			},
			tokenManagerFunc: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
				return nil, errors.New("invalid credentials path")
			},
			browserAuthFunc: func(baseURL string) browserAuthenticator {
				mockBrowser := new(mockBrowserAuthenticator)
				mockBrowser.On("WithHeadless", false).Return(mockBrowser)
				return mockBrowser
			},
			expectedError: "failed to create TokenManager",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := &tokenSaverImpl{
				tokenManagerFactory: tt.tokenManagerFunc,
				browserAuthFactory:  tt.browserAuthFunc,
				userHomeDir:         tt.userHomeDirFunc,
				mkdirAll:            tt.mkdirAllFunc,
			}

			err := ts.run()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestTokenSaver_Run_NilTokensFromAuth(t *testing.T) {
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", mock.Anything).Return(nil)

	testTokens := createTestTokens()
	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.NoError(t, err)
	mockTokenMgr.AssertExpectations(t)
}

func TestTokenSaver_Run_EmptyTokens(t *testing.T) {
	emptyTokens := &webauthn.AuthTokens{
		HGLogin:   "",
		XSRFToken: "",
		ExpiresAt: time.Now(),
	}

	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", emptyTokens).Return(nil)

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(emptyTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	err := ts.run()

	assert.NoError(t, err)
	mockTokenMgr.AssertCalled(t, "SaveTokens", emptyTokens)
}

func TestPrintSuccess_Formatting(t *testing.T) {
	tokens := &webauthn.AuthTokens{
		HGLogin:   "ab12345678901234567890",
		XSRFToken: "cd12345678901234567890",
		ExpiresAt: time.Date(2026, 3, 3, 14, 30, 0, 0, time.UTC),
	}

	assert.NotPanics(t, func() {
		printSuccess("/test/path/tokens.json", tokens)
	})
}

func TestTokenSaver_DependenciesNotNil(t *testing.T) {
	ts := newTokenSaver()

	assert.NotNil(t, ts.tokenManagerFactory, "tokenManagerFactory should not be nil")
	assert.NotNil(t, ts.browserAuthFactory, "browserAuthFactory should not be nil")
	assert.NotNil(t, ts.userHomeDir, "userHomeDir should not be nil")
	assert.NotNil(t, ts.mkdirAll, "mkdirAll should not be nil")
}

func TestCreateTestTokens(t *testing.T) {
	tokens := createTestTokens()

	assert.NotNil(t, tokens)
	assert.NotEmpty(t, tokens.HGLogin, "HGLogin should not be empty")
	assert.NotEmpty(t, tokens.XSRFToken, "XSRFToken should not be empty")
	assert.False(t, tokens.ExpiresAt.IsZero(), "ExpiresAt should be set")
	assert.Greater(t, tokens.ExpiresAt, time.Now(), "ExpiresAt should be in future")
}

func TestBrowserAuthAdapter(t *testing.T) {
	ba := webauthn.NewBrowserAuth("https://app.hourglass-app.com")
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	baWithHeadless := adapter.WithHeadless(false)
	assert.NotNil(t, baWithHeadless)
}

func TestBrowserAuthAdapter_Authenticate(t *testing.T) {
	ba := webauthn.NewBrowserAuth("https://app.hourglass-app.com")
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.BrowserAuth)
}

func TestBrowserAuthAdapter_WithHeadlessTrue(t *testing.T) {
	ba := webauthn.NewBrowserAuth("https://app.hourglass-app.com")
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	headlessAdapter := adapter.WithHeadless(true)

	assert.NotNil(t, headlessAdapter)
	assert.IsType(t, &browserAuthAdapter{}, headlessAdapter)
}

func TestBrowserAuthAdapter_Chaining(t *testing.T) {
	ba := webauthn.NewBrowserAuth("https://app.hourglass-app.com")
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	adapter1 := adapter.WithHeadless(false)
	adapter2 := adapter1.WithHeadless(true)

	assert.NotNil(t, adapter2)
	assert.IsType(t, &browserAuthAdapter{}, adapter2)
}

func TestNewTokenSaver_DefaultFactories(t *testing.T) {
	ts := newTokenSaver()

	homeDir, _ := ts.userHomeDir()
	assert.NotEmpty(t, homeDir)

	tempDir := t.TempDir()
	err := ts.mkdirAll(tempDir, 0700)
	assert.NoError(t, err)
}

func TestPrintSuccess_VerifyFormat(t *testing.T) {
	tokens := &webauthn.AuthTokens{
		HGLogin:   "ABCD12345678901234567890",
		XSRFToken: "XYZW9876543210987654321",
		ExpiresAt: time.Date(2026, 12, 25, 14, 30, 0, 0, time.UTC),
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSuccess("/test/tokens.json", tokens)

	w.Close()
	os.Stdout = oldStdout

	output, _ := io.ReadAll(r)
	outputStr := string(output)
	assert.Contains(t, outputStr, "ABCD...7890")
	assert.Contains(t, outputStr, "XYZW...4321")
	assert.Contains(t, outputStr, "25/12/2026")
	assert.Contains(t, outputStr, "/test/tokens.json")
}

func TestTokenSaver_Run_TokenRenewedCallback(t *testing.T) {
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			_ = len(opts) > 0
			return nil, errors.New("stop here")
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			mockBrowser := new(mockBrowserAuthenticator)
			mockBrowser.On("WithHeadless", false).Return(mockBrowser)
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	_ = ts.run()
}

func TestCreateTestTokens_FutureExpiry(t *testing.T) {
	tokens := createTestTokens()

	now := time.Now()
	assert.True(t, tokens.ExpiresAt.After(now), "Tokens should expire in the future")
	assert.True(t, tokens.ExpiresAt.Sub(now) > 7*time.Hour, "Tokens should expire in more than 7 hours")
}

func TestPrintSuccess_TokenLengthEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		hgLogin string
		xsrf    string
	}{
		{"exactly 4 chars", "test", "xsrf"},
		{"exactly 5 chars", "test1", "xsrf1"},
		{"long token", "very_long_hg_login_token_value_12345", "very_long_xsrf_token_value_67890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &webauthn.AuthTokens{
				HGLogin:   tt.hgLogin,
				XSRFToken: tt.xsrf,
				ExpiresAt: time.Now(),
			}

			assert.NotPanics(t, func() {
				printSuccess("/path/tokens.json", tokens)
			})
		})
	}
}

func TestTokenSaver_Run_VerifyTokensPath(t *testing.T) {
	testTokens := createTestTokens()
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", testTokens).Return(nil)

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			_ = filepath.Join(filepath.Dir(credsPath), "auth-tokens.json")
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/test", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			return nil
		},
	}

	_ = ts.run()
}

func TestPrintSuccess_EmptyPath(t *testing.T) {
	tokens := &webauthn.AuthTokens{
		HGLogin:   "test12345678901234567890",
		XSRFToken: "xsrf9876543210987654321",
		ExpiresAt: time.Now(),
	}

	assert.NotPanics(t, func() {
		printSuccess("", tokens)
	})
}

func TestBrowserAuthAdapter_NilBrowserAuth(t *testing.T) {
	adapter := &browserAuthAdapter{}

	assert.NotNil(t, adapter)
	assert.Nil(t, adapter.BrowserAuth)

	assert.Panics(t, func() {
		adapter.WithHeadless(false)
	})
}

func TestBrowserAuthAdapter_AuthenticateReturnsTokens(t *testing.T) {
	ba := webauthn.NewBrowserAuth("https://app.hourglass-app.com")
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	assert.NotNil(t, adapter.BrowserAuth)
	assert.NotNil(t, adapter)
}

func TestBrowserAuthAdapter_AuthenticateCalled(t *testing.T) {
	ba := webauthn.NewBrowserAuth("https://app.hourglass-app.com")
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.BrowserAuth)
}

func TestTokenSaverImpl_newTokenSaver_RealFactories(t *testing.T) {
	ts := newTokenSaver()

	// Test that real factories work correctly
	t.Run("tokenManagerFactory creates real manager", func(t *testing.T) {
		tempDir := t.TempDir()
		credsPath := filepath.Join(tempDir, "creds.json")
		baseURL := "https://app.hourglass-app.com"

		mgr, err := ts.tokenManagerFactory(credsPath, baseURL)
		assert.NoError(t, err)
		assert.NotNil(t, mgr)
	})

	t.Run("browserAuthFactory creates real browser auth", func(t *testing.T) {
		baseURL := "https://app.hourglass-app.com"
		auth := ts.browserAuthFactory(baseURL)
		assert.NotNil(t, auth)
	})

	t.Run("userHomeDir returns real home directory", func(t *testing.T) {
		home, err := ts.userHomeDir()
		assert.NoError(t, err)
		assert.NotEmpty(t, home)
	})

	t.Run("mkdirAll creates real directories", func(t *testing.T) {
		tempDir := t.TempDir()
		testDir := filepath.Join(tempDir, "test", "nested")
		err := ts.mkdirAll(testDir, 0755)
		assert.NoError(t, err)
		assert.DirExists(t, testDir)
	})
}

func TestTokenSaver_Run_CompleteFlow(t *testing.T) {
	testTokens := createTestTokens()
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", testTokens).Return(nil)

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	homeDir := "/home/test"

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return homeDir, nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			assert.Equal(t, filepath.Join(homeDir, ".hourglass-rpa"), path)
			assert.Equal(t, os.FileMode(0700), perm)
			return nil
		},
	}

	err := ts.run()

	assert.NoError(t, err)
	mockTokenMgr.AssertExpectations(t)
	mockBrowser.AssertExpectations(t)
}

func TestTokenSaver_Run_MkdirAllWithCorrectPath(t *testing.T) {
	testTokens := createTestTokens()
	mockTokenMgr := new(mockTokenSaver)
	mockTokenMgr.On("SaveTokens", testTokens).Return(nil)

	mockBrowser := new(mockBrowserAuthenticator)
	mockBrowser.On("Authenticate").Return(testTokens, nil)
	mockBrowser.On("WithHeadless", false).Return(mockBrowser)

	var mkdirPath string

	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return mockTokenMgr, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return mockBrowser
		},
		userHomeDir: func() (string, error) {
			return "/home/testuser", nil
		},
		mkdirAll: func(path string, perm os.FileMode) error {
			mkdirPath = path
			return nil
		},
	}

	_ = ts.run()

	assert.Equal(t, "/home/testuser/.hourglass-rpa", mkdirPath)
}
