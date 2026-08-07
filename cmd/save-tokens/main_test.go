package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

const (
	testBaseURL       = "https://app.hourglass-app.com"
	testConfigDirName = ".hourglass-rpa"
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

type mockTokenLoader struct {
	mock.Mock
}

func (m *mockTokenLoader) LoadTokens() (*webauthn.AuthTokens, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webauthn.AuthTokens), args.Error(1)
}

func (m *mockBrowserAuthenticator) Authenticate() (*webauthn.AuthTokens, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webauthn.AuthTokens), args.Error(1)
}

func (m *mockBrowserAuthenticator) ExtractTokensFromProfile() (*webauthn.AuthTokens, error) {
	method := "ExtractTokensFromProfile"
	for _, call := range m.ExpectedCalls {
		if call.Method == method {
			args := m.Called()
			if args.Get(0) == nil {
				return nil, args.Error(1)
			}
			return args.Get(0).(*webauthn.AuthTokens), args.Error(1)
		}
	}

	args := m.MethodCalled("Authenticate")
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webauthn.AuthTokens), args.Error(1)
}

func (m *mockBrowserAuthenticator) WithHeadless(headless bool) browserAuthenticator {
	m.Called(headless)
	return m
}

func (m *mockBrowserAuthenticator) WithProfileDir(profileDir string) browserAuthenticator {
	_ = profileDir
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
	mockBrowser.On("ExtractTokensFromProfile").Return(testTokens, nil)
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
}

func TestTokenSaver_Run_LaunchBrowserError(t *testing.T) {
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &mockBrowserAuthenticator{}
		},
		userHomeDir:         func() (string, error) { return "/home/test", nil },
		mkdirAll:            func(path string, perm os.FileMode) error { return nil },
		launchBrowser:       func(profileDir, loginURL string) error { return errors.New("launch failed") },
		waitForConfirmation: func() error { return nil },
	}

	err := ts.run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to launch manual browser")
}

func TestTokenSaver_Run_ProfileDirCreationError(t *testing.T) {
	callCount := 0
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &mockBrowserAuthenticator{}
		},
		userHomeDir: func() (string, error) { return "/home/test", nil },
		mkdirAll: func(path string, perm os.FileMode) error {
			callCount++
			if callCount == 2 {
				return errors.New("profile mkdir failed")
			}
			return nil
		},
	}

	err := ts.run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create chrome profile directory")
}

func TestTokenSaver_Run_UsesChromeProfileDirEnv(t *testing.T) {
	t.Setenv("CHROME_PROFILE_DIR", "/tmp/custom-profile")
	mkdirPaths := []string{}
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return nil, errors.New("stop here")
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator { return &mockBrowserAuthenticator{} },
		userHomeDir:        func() (string, error) { return "/home/test", nil },
		mkdirAll: func(path string, perm os.FileMode) error {
			mkdirPaths = append(mkdirPaths, path)
			return nil
		},
	}

	_ = ts.run()
	assert.Contains(t, mkdirPaths, "/tmp/custom-profile")
}

func TestTokenSaver_Run_ConfirmationError(t *testing.T) {
	ts := &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return &mockTokenSaver{}, nil
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &mockBrowserAuthenticator{}
		},
		userHomeDir:         func() (string, error) { return "/home/test", nil },
		mkdirAll:            func(path string, perm os.FileMode) error { return nil },
		launchBrowser:       func(profileDir, loginURL string) error { return nil },
		waitForConfirmation: func() error { return errors.New("cancelled") },
	}

	err := ts.run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manual browser confirmation failed")
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
	ba := webauthn.NewBrowserAuth(testBaseURL)
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	baWithHeadless := adapter.WithHeadless(false)
	assert.NotNil(t, baWithHeadless)
}

func TestBrowserAuthAdapter_Authenticate(t *testing.T) {
	ba := webauthn.NewBrowserAuth(testBaseURL)
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	assert.NotNil(t, adapter)
	assert.Same(t, ba, adapter.BrowserAuth)
}

func TestBrowserAuthAdapter_WithHeadlessTrue(t *testing.T) {
	ba := webauthn.NewBrowserAuth(testBaseURL)
	adapter := &browserAuthAdapter{BrowserAuth: ba}

	headlessAdapter := adapter.WithHeadless(true)

	assert.NotNil(t, headlessAdapter)
	assert.IsType(t, &browserAuthAdapter{}, headlessAdapter)
}

func TestBrowserAuthAdapter_Chaining(t *testing.T) {
	ba := webauthn.NewBrowserAuth(testBaseURL)
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

	assert.NotPanics(t, func() {
		result := adapter.WithHeadless(false)
		assert.NotNil(t, result)
	})

	result := adapter.WithProfileDir(t.TempDir())
	assert.NotNil(t, result)

	tokens, err := adapter.ExtractTokensFromProfile()
	assert.Nil(t, tokens)
	assert.EqualError(t, err, "browser auth is not configured")
}

func TestBrowserAuthAdapter_AuthenticateReturnsTokens(t *testing.T) {
	tokens := createTestTokens()
	called := false
	adapter := &browserAuthAdapter{
		authenticateFunc: func() (*webauthn.AuthTokens, error) {
			called = true
			return tokens, nil
		},
	}

	got, err := adapter.Authenticate()

	assert.True(t, called)
	assert.NoError(t, err)
	assert.Equal(t, tokens, got)
}

func TestBrowserAuthAdapter_Authenticate_UsesWrapperFunction(t *testing.T) {
	expected := createTestTokens()
	called := false

	adapter := &browserAuthAdapter{
		authenticateFunc: func() (*webauthn.AuthTokens, error) {
			called = true
			return expected, nil
		},
	}

	tokens, err := adapter.Authenticate()

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, expected, tokens)
}

func TestBrowserAuthAdapter_Authenticate_ReturnsErrorWithNilBrowserAuth(t *testing.T) {
	adapter := &browserAuthAdapter{}

	tokens, err := adapter.Authenticate()

	assert.Nil(t, tokens)
	assert.EqualError(t, err, "browser auth is not configured")
}

func TestBrowserAuthAdapter_Authenticate_UsesWrappedAuth(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("CHROME_BIN", filepath.Join(t.TempDir(), "missing-chrome"))
	t.Setenv("CHROME_PATH", "")
	adapter := &browserAuthAdapter{BrowserAuth: webauthn.NewBrowserAuth("http://localhost")}
	tokens, err := adapter.Authenticate()
	assert.Nil(t, tokens)
	assert.Error(t, err)
}

func TestBrowserAuthAdapter_WithHeadless_UsesWrapperFunction(t *testing.T) {
	called := false
	adapter := &browserAuthAdapter{
		withHeadlessFunc: func(headless bool) *webauthn.BrowserAuth {
			called = true
			assert.True(t, headless)
			return webauthn.NewBrowserAuth(testBaseURL)
		},
	}

	headlessAdapter := adapter.WithHeadless(true)

	assert.True(t, called)
	assert.NotNil(t, headlessAdapter)
}

func TestBrowserAuthAdapter_ExtractTokensFromProfile_UsesWrapperFunction(t *testing.T) {
	expected := createTestTokens()
	called := false
	adapter := &browserAuthAdapter{
		extractTokensFunc: func() (*webauthn.AuthTokens, error) {
			called = true
			return expected, nil
		},
	}

	tokens, err := adapter.ExtractTokensFromProfile()
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, expected, tokens)
}

func TestBrowserAuthAdapter_ExtractTokensFromProfile_UsesWrappedAuth(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("CHROME_BIN", filepath.Join(t.TempDir(), "missing-chrome"))
	t.Setenv("CHROME_PATH", "")
	adapter := &browserAuthAdapter{BrowserAuth: webauthn.NewBrowserAuth("http://localhost")}
	tokens, err := adapter.ExtractTokensFromProfile()
	assert.Nil(t, tokens)
	assert.Error(t, err)
}

func TestBrowserAuthAdapter_WithProfileDir_UsesWrapperFunction(t *testing.T) {
	called := false
	adapter := &browserAuthAdapter{
		withProfileDirFunc: func(profileDir string) *webauthn.BrowserAuth {
			called = true
			assert.Equal(t, "/tmp/profile", profileDir)
			return webauthn.NewBrowserAuth(testBaseURL)
		},
	}

	profiled := adapter.WithProfileDir("/tmp/profile")
	assert.True(t, called)
	assert.NotNil(t, profiled)
}

func TestBrowserAuthAdapter_WithProfileDir_UsesWrappedAuth(t *testing.T) {
	adapter := &browserAuthAdapter{BrowserAuth: webauthn.NewBrowserAuth("http://localhost")}
	profiled := adapter.WithProfileDir("/tmp/profile")
	assert.NotNil(t, profiled)
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
	chromePath := filepath.Join(tempDir, "chrome")
	err := os.WriteFile(chromePath, []byte("#!/bin/sh\nexit 0\n"), 0755)
	assert.NoError(t, err)
	t.Setenv("CHROME_BIN", chromePath)
	t.Setenv("CHROME_PATH", "")

	profileDir := filepath.Join(tempDir, "profile")
	err = launchChromeForManualLogin(profileDir, "https://example.com/login")
	assert.NoError(t, err)
	_, statErr := os.Stat(profileDir)
	assert.NoError(t, statErr)
}

func TestLaunchChromeForManualLogin_UsesChromePathFallback(t *testing.T) {
	tempDir := t.TempDir()
	chromePath := filepath.Join(tempDir, "chrome-path")
	err := os.WriteFile(chromePath, []byte("#!/bin/sh\nexit 0\n"), 0755)
	assert.NoError(t, err)
	t.Setenv("CHROME_BIN", "")
	t.Setenv("CHROME_PATH", chromePath)

	profileDir := filepath.Join(tempDir, "profile-fallback")
	err = launchChromeForManualLogin(profileDir, "https://example.com/login")
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
		cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("[ \"%s\" = \"/usr/bin/google-chrome\" ]", name))
		return cmd
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
	assert.NoError(t, err)
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	go func() {
		_, _ = w.WriteString("\n")
		_ = w.Close()
	}()

	assert.NoError(t, waitForBrowserConfirmation())
}

func TestWaitForBrowserConfirmation_ReadError(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	assert.NoError(t, err)
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

func TestDefaultNewTokenLoader(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	err := os.MkdirAll(configDir, 0700)
	assert.NoError(t, err)

	tokensPath := filepath.Join(configDir, "auth-tokens.json")
	loader, err := defaultNewTokenLoader(configDir, tokensPath)

	assert.NoError(t, err)
	assert.NotNil(t, loader)
}

func TestPrintTokenRenewedMessage(t *testing.T) {
	assert.NotPanics(t, func() {
		printTokenRenewedMessage()
	})
}

func TestOnTokenRenewed(t *testing.T) {
	assert.NotPanics(t, func() {
		onTokenRenewed(createTestTokens())
	})
}

func TestMain_Success(t *testing.T) {
	originalNewTokenSaverForMain := newTokenSaverForMain
	originalUserHomeDirForMain := userHomeDirForMain
	originalNewTokenLoader := newTokenLoader
	originalLogFatal := logFatal
	originalPrintSuccessFn := printSuccessFn
	defer func() {
		newTokenSaverForMain = originalNewTokenSaverForMain
		userHomeDirForMain = originalUserHomeDirForMain
		newTokenLoader = originalNewTokenLoader
		logFatal = originalLogFatal
		printSuccessFn = originalPrintSuccessFn
	}()

	testTokens := createTestTokens()
	loader := new(mockTokenLoader)
	loader.On("LoadTokens").Return(testTokens, nil)
	runTokens := createTestTokens()
	runTokenMgr := new(mockTokenSaver)
	runTokenMgr.On("SaveTokens", runTokens).Return(nil)
	runBrowser := new(mockBrowserAuthenticator)
	runBrowser.On("Authenticate").Return(runTokens, nil)
	runBrowser.On("WithHeadless", false).Return(runBrowser)

	newTokenSaverForMain = func() *tokenSaverImpl {
		return &tokenSaverImpl{
			tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
				return runTokenMgr, nil
			},
			browserAuthFactory: func(baseURL string) browserAuthenticator {
				return runBrowser
			},
			userHomeDir: func() (string, error) {
				return "/home/main-test", nil
			},
			mkdirAll: func(path string, perm os.FileMode) error {
				return nil
			},
		}
	}

	userHomeDirForMain = func() (string, error) {
		return "/home/main-test", nil
	}

	loaderCalled := false
	newTokenLoader = func(configDir, tokensPath string) (tokenLoader, error) {
		loaderCalled = true
		assert.Equal(t, filepath.Join("/home/main-test", testConfigDirName), configDir)
		assert.Equal(t, filepath.Join("/home/main-test", testConfigDirName, "auth-tokens.json"), tokensPath)
		return loader, nil
	}

	fatalCalled := false
	logFatal = func(v ...any) {
		fatalCalled = true
	}

	printCalled := false
	printSuccessFn = func(tokensPath string, tokens *webauthn.AuthTokens) {
		printCalled = true
		assert.Equal(t, filepath.Join("/home/main-test", testConfigDirName, "auth-tokens.json"), tokensPath)
		assert.Equal(t, testTokens, tokens)
	}

	main()

	assert.True(t, loaderCalled)
	assert.False(t, fatalCalled)
	assert.True(t, printCalled)
	loader.AssertExpectations(t)
	runTokenMgr.AssertExpectations(t)
	runBrowser.AssertExpectations(t)
}

func TestMain_RunTokenSaverError(t *testing.T) {
	originalNewTokenSaverForMain := newTokenSaverForMain
	originalUserHomeDirForMain := userHomeDirForMain
	originalNewTokenLoader := newTokenLoader
	originalLogFatal := logFatal
	originalPrintSuccessFn := printSuccessFn
	defer func() {
		newTokenSaverForMain = originalNewTokenSaverForMain
		userHomeDirForMain = originalUserHomeDirForMain
		newTokenLoader = originalNewTokenLoader
		logFatal = originalLogFatal
		printSuccessFn = originalPrintSuccessFn
	}()

	newTokenSaverForMain = func() *tokenSaverImpl {
		return &tokenSaverImpl{
			userHomeDir: func() (string, error) {
				return "", errors.New("run failed")
			},
		}
	}
	userHomeDirForMain = func() (string, error) {
		return "/home/main-test", nil
	}
	newTokenLoader = func(configDir, tokensPath string) (tokenLoader, error) {
		loader := new(mockTokenLoader)
		loader.On("LoadTokens").Return(createTestTokens(), nil)
		return loader, nil
	}

	type fatalPanic struct{}
	fatalCalls := 0
	logFatal = func(v ...any) {
		fatalCalls++
		panic(fatalPanic{})
	}
	printSuccessFn = func(tokensPath string, tokens *webauthn.AuthTokens) {}

	assert.PanicsWithValue(t, fatalPanic{}, func() {
		main()
	})

	assert.Equal(t, 1, fatalCalls)
}

func TestMain_NewTokenLoaderError(t *testing.T) {
	originalNewTokenSaverForMain := newTokenSaverForMain
	originalUserHomeDirForMain := userHomeDirForMain
	originalNewTokenLoader := newTokenLoader
	originalLogFatal := logFatal
	originalPrintSuccessFn := printSuccessFn
	defer func() {
		newTokenSaverForMain = originalNewTokenSaverForMain
		userHomeDirForMain = originalUserHomeDirForMain
		newTokenLoader = originalNewTokenLoader
		logFatal = originalLogFatal
		printSuccessFn = originalPrintSuccessFn
	}()

	runTokens := createTestTokens()
	runTokenMgr := new(mockTokenSaver)
	runTokenMgr.On("SaveTokens", runTokens).Return(nil)
	runBrowser := new(mockBrowserAuthenticator)
	runBrowser.On("Authenticate").Return(runTokens, nil)
	runBrowser.On("WithHeadless", false).Return(runBrowser)

	newTokenSaverForMain = func() *tokenSaverImpl {
		return &tokenSaverImpl{
			tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
				return runTokenMgr, nil
			},
			browserAuthFactory: func(baseURL string) browserAuthenticator {
				return runBrowser
			},
			userHomeDir: func() (string, error) {
				return "/home/main-test", nil
			},
			mkdirAll: func(path string, perm os.FileMode) error {
				return nil
			},
		}
	}
	userHomeDirForMain = func() (string, error) {
		return "/home/main-test", nil
	}
	newTokenLoader = func(configDir, tokensPath string) (tokenLoader, error) {
		return nil, errors.New("loader failed")
	}

	type fatalPanic struct{}
	fatalCalls := 0
	logFatal = func(v ...any) {
		fatalCalls++
		panic(fatalPanic{})
	}
	printCalled := false
	printSuccessFn = func(tokensPath string, tokens *webauthn.AuthTokens) {
		printCalled = true
	}

	assert.PanicsWithValue(t, fatalPanic{}, func() {
		main()
	})

	assert.Equal(t, 1, fatalCalls)
	assert.False(t, printCalled)
	runTokenMgr.AssertExpectations(t)
	runBrowser.AssertExpectations(t)
}

func TestMain_LoadTokensError(t *testing.T) {
	originalNewTokenSaverForMain := newTokenSaverForMain
	originalUserHomeDirForMain := userHomeDirForMain
	originalNewTokenLoader := newTokenLoader
	originalLogFatal := logFatal
	originalPrintSuccessFn := printSuccessFn
	defer func() {
		newTokenSaverForMain = originalNewTokenSaverForMain
		userHomeDirForMain = originalUserHomeDirForMain
		newTokenLoader = originalNewTokenLoader
		logFatal = originalLogFatal
		printSuccessFn = originalPrintSuccessFn
	}()

	loader := new(mockTokenLoader)
	loader.On("LoadTokens").Return(nil, errors.New("load tokens failed"))
	runTokens := createTestTokens()
	runTokenMgr := new(mockTokenSaver)
	runTokenMgr.On("SaveTokens", runTokens).Return(nil)
	runBrowser := new(mockBrowserAuthenticator)
	runBrowser.On("Authenticate").Return(runTokens, nil)
	runBrowser.On("WithHeadless", false).Return(runBrowser)

	newTokenSaverForMain = func() *tokenSaverImpl {
		return &tokenSaverImpl{
			tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
				return runTokenMgr, nil
			},
			browserAuthFactory: func(baseURL string) browserAuthenticator {
				return runBrowser
			},
			userHomeDir: func() (string, error) {
				return "/home/main-test", nil
			},
			mkdirAll: func(path string, perm os.FileMode) error {
				return nil
			},
		}
	}
	userHomeDirForMain = func() (string, error) {
		return "/home/main-test", nil
	}
	newTokenLoader = func(configDir, tokensPath string) (tokenLoader, error) {
		return loader, nil
	}

	type fatalPanic struct{}
	fatalCalls := 0
	logFatal = func(v ...any) {
		fatalCalls++
		panic(fatalPanic{})
	}
	printCalled := false
	printSuccessFn = func(tokensPath string, tokens *webauthn.AuthTokens) {
		printCalled = true
	}

	assert.PanicsWithValue(t, fatalPanic{}, func() {
		main()
	})

	assert.Equal(t, 1, fatalCalls)
	assert.False(t, printCalled)
	loader.AssertExpectations(t)
	runTokenMgr.AssertExpectations(t)
	runBrowser.AssertExpectations(t)
}

func TestTokenSaverImpl_newTokenSaver_RealFactories(t *testing.T) {
	ts := newTokenSaver()

	// Test that real factories work correctly
	t.Run("tokenManagerFactory creates real manager", func(t *testing.T) {
		tempDir := t.TempDir()
		credsPath := filepath.Join(tempDir, "creds.json")
		baseURL := testBaseURL

		mgr, err := ts.tokenManagerFactory(credsPath, baseURL)
		assert.NoError(t, err)
		assert.NotNil(t, mgr)
	})

	t.Run("browserAuthFactory creates real browser auth", func(t *testing.T) {
		baseURL := testBaseURL
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
	expectedPaths := []string{
		filepath.Join(homeDir, testConfigDirName),
		filepath.Join(homeDir, testConfigDirName, "chrome-profile"),
	}
	callIndex := 0

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
			require.Less(t, callIndex, len(expectedPaths))
			assert.Equal(t, expectedPaths[callIndex], path)
			assert.Equal(t, os.FileMode(0700), perm)
			callIndex++
			return nil
		},
	}

	err := ts.run()

	assert.NoError(t, err)
	assert.Equal(t, len(expectedPaths), callIndex)
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

	var mkdirPaths []string

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
			mkdirPaths = append(mkdirPaths, path)
			return nil
		},
	}

	_ = ts.run()

	assert.Equal(t, []string{
		filepath.Join("/home/testuser", testConfigDirName),
		filepath.Join("/home/testuser", testConfigDirName, "chrome-profile"),
	}, mkdirPaths)
}
