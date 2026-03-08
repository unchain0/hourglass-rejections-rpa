package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/auth/webauthn"
	"hourglass-rejections-rpa/internal/testutil"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTokenRefresher_Run_Success(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.HomeDir = "/home/test"
	mockFS.Files["/home/test/.hourglass-rpa/auth-tokens.json"] = []byte(`{
		"hg_login": "test-token",
		"xsrf_token": "test-xsrf",
		"expires_at": "2026-03-04T00:00:00Z"
	}`)

	mockHTTP := testutil.NewMockHTTPClient()
	mockHTTP.Response = &http.Response{
		StatusCode: 200,
		Body:       http.NoBody,
		Header:     http.Header{"Set-Cookie": []string{}},
	}

	tr := &tokenRefresher{
		fs:         mockFS,
		httpClient: mockHTTP,
		baseURL:    "https://app.hourglass-app.com",
	}

	err := tr.Run()
	assert.NoError(t, err)
	assert.Len(t, mockFS.Calls.WriteFile, 1)
}

func TestTokenRefresher_Run_HomeDirError(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.HomeDirErr = errors.New("home dir error")

	tr := &tokenRefresher{
		fs: mockFS,
	}

	err := tr.Run()
	assert.Error(t, err)
}

func TestTokenRefresher_loadTokens_JSONParseError(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.Files["/tmp/tokens.json"] = []byte(`{"invalid json`)

	tr := &tokenRefresher{fs: mockFS}
	_, err := tr.loadTokens("/tmp/tokens.json")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "erro ao ler tokens")
}

func TestOsFileSystem(t *testing.T) {
	fs := &osFileSystem{}

	t.Run("UserHomeDir returns home directory", func(t *testing.T) {
		home, err := fs.UserHomeDir()
		assert.NoError(t, err)
		assert.NotEmpty(t, home)
	})

	t.Run("ReadFile reads existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		testData := []byte("hello world")
		err := os.WriteFile(testFile, testData, 0644)
		require.NoError(t, err)

		data, err := fs.ReadFile(testFile)
		assert.NoError(t, err)
		assert.Equal(t, testData, data)
	})

	t.Run("WriteFile writes file", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		testData := []byte("hello world")

		err := fs.WriteFile(testFile, testData, 0644)
		assert.NoError(t, err)

		readData, err := os.ReadFile(testFile)
		assert.NoError(t, err)
		assert.Equal(t, testData, readData)
	})

	t.Run("MkdirAll creates directories", func(t *testing.T) {
		tempDir := t.TempDir()
		testDir := filepath.Join(tempDir, "test", "nested")

		err := fs.MkdirAll(testDir, 0755)
		assert.NoError(t, err)
		assert.DirExists(t, testDir)
	})
}

func TestTokenRefresher_tryRefresh_WithCookies(t *testing.T) {
	t.Run("receives new tokens from cookies", func(t *testing.T) {
		mockHTTP := testutil.NewMockHTTPClient()
		mockHTTP.Response = &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
			Header: http.Header{
				"Set-Cookie": []string{
					"hglogin=new-hglogin-value; Path=/",
					"X-Hourglass-XSRF-Token=new-xsrf-value; Path=/",
				},
			},
		}

		tr := &tokenRefresher{
			httpClient: mockHTTP,
			baseURL:    "https://app.hourglass-app.com",
		}

		tokens := &webauthn.AuthTokens{
			HGLogin:   "old-hglogin",
			XSRFToken: "old-xsrf",
		}

		newTokens, err := tr.tryRefresh(tokens)
		require.NoError(t, err)
		assert.Equal(t, "new-hglogin-value", newTokens.HGLogin)
		assert.Equal(t, "new-xsrf-value", newTokens.XSRFToken)
	})
}

func TestTokenRefresher_saveTokens_MkdirError(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.MkdirErr = errors.New("mkdir error")

	tr := &tokenRefresher{fs: mockFS}
	tokens := &webauthn.AuthTokens{
		HGLogin:   "test",
		XSRFToken: "xsrf",
	}

	err := tr.saveTokens("/tmp/test/tokens.json", tokens)
	assert.Error(t, err)
}

func TestNewTokenRefresher_Defaults(t *testing.T) {
	tr := newTokenRefresher()
	assert.NotNil(t, tr.fs)
	assert.NotNil(t, tr.httpClient)
	assert.Equal(t, "https://app.hourglass-app.com", tr.baseURL)
}

func TestTokenRefresher_loadTokens(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockFS := testutil.NewMockFileSystem()
		mockFS.Files["/tmp/tokens.json"] = []byte(`{
			"hg_login": "test",
			"xsrf_token": "xsrf",
			"expires_at": "2026-03-04T00:00:00Z"
		}`)

		tr := &tokenRefresher{fs: mockFS}
		tokens, err := tr.loadTokens("/tmp/tokens.json")

		require.NoError(t, err)
		assert.Equal(t, "test", tokens.HGLogin)
	})

	t.Run("file not found", func(t *testing.T) {
		mockFS := testutil.NewMockFileSystem()
		mockFS.ReadErr = errors.New("not found")

		tr := &tokenRefresher{fs: mockFS}
		_, err := tr.loadTokens("/tmp/tokens.json")

		assert.Error(t, err)
	})
}

func TestNewTokenRefresher(t *testing.T) {
	tr := newTokenRefresher()
	assert.NotNil(t, tr)
	assert.NotNil(t, tr.fs)
	assert.NotNil(t, tr.httpClient)
}

func TestTokenRefresher_tryRefresh(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockHTTP := testutil.NewMockHTTPClient()
		mockHTTP.Response = &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
			Header:     http.Header{},
		}

		tr := &tokenRefresher{
			httpClient: mockHTTP,
			baseURL:    "https://app.hourglass-app.com",
		}

		tokens := &webauthn.AuthTokens{
			HGLogin:   "test-token",
			XSRFToken: "test-xsrf",
		}

		newTokens, err := tr.tryRefresh(tokens)
		require.NoError(t, err)
		assert.Equal(t, "test-token", newTokens.HGLogin)
		assert.Equal(t, "test-xsrf", newTokens.XSRFToken)
	})

	t.Run("http error", func(t *testing.T) {
		mockHTTP := testutil.NewMockHTTPClient()
		mockHTTP.Err = errors.New("network error")

		tr := &tokenRefresher{
			httpClient: mockHTTP,
			baseURL:    "https://app.hourglass-app.com",
		}

		tokens := &webauthn.AuthTokens{}
		_, err := tr.tryRefresh(tokens)

		assert.Error(t, err)
	})

	t.Run("non-200 status", func(t *testing.T) {
		mockHTTP := testutil.NewMockHTTPClient()
		mockHTTP.Response = testutil.MockErrorResponse(401, "unauthorized")

		tr := &tokenRefresher{
			httpClient: mockHTTP,
			baseURL:    "https://app.hourglass-app.com",
		}

		tokens := &webauthn.AuthTokens{}
		_, err := tr.tryRefresh(tokens)

		assert.Error(t, err)
	})

	t.Run("request creation error", func(t *testing.T) {
		tr := &tokenRefresher{
			httpClient: testutil.NewMockHTTPClient(),
			baseURL:    "http://bad\nurl",
		}

		tokens := &webauthn.AuthTokens{}
		_, err := tr.tryRefresh(tokens)

		assert.Error(t, err)
	})

	t.Run("ignores empty cookie values", func(t *testing.T) {
		mockHTTP := testutil.NewMockHTTPClient()
		mockHTTP.Response = &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
			Header: http.Header{
				"Set-Cookie": []string{
					"hglogin=; Path=/",
					"X-Hourglass-XSRF-Token=; Path=/",
				},
			},
		}

		tr := &tokenRefresher{
			httpClient: mockHTTP,
			baseURL:    "https://app.hourglass-app.com",
		}

		tokens := &webauthn.AuthTokens{
			HGLogin:   "old-hglogin",
			XSRFToken: "old-xsrf",
		}

		newTokens, err := tr.tryRefresh(tokens)
		require.NoError(t, err)
		assert.Equal(t, "old-hglogin", newTokens.HGLogin)
		assert.Equal(t, "old-xsrf", newTokens.XSRFToken)
	})
}

func TestTokenRefresher_saveTokens(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockFS := testutil.NewMockFileSystem()

		tr := &tokenRefresher{fs: mockFS}
		tokens := &webauthn.AuthTokens{
			HGLogin:   "test",
			XSRFToken: "xsrf",
		}

		err := tr.saveTokens("/tmp/test/tokens.json", tokens)
		require.NoError(t, err)
		assert.Len(t, mockFS.Calls.MkdirAll, 1)
		assert.Len(t, mockFS.Calls.WriteFile, 1)
	})

	t.Run("write error", func(t *testing.T) {
		mockFS := testutil.NewMockFileSystem()
		mockFS.WriteErr = errors.New("write error")

		tr := &tokenRefresher{fs: mockFS}
		tokens := &webauthn.AuthTokens{}

		err := tr.saveTokens("/tmp/test/tokens.json", tokens)
		assert.Error(t, err)
	})

	t.Run("marshal error", func(t *testing.T) {
		oldMarshal := jsonMarshal
		jsonMarshal = func(v any) ([]byte, error) {
			return nil, errors.New("marshal error")
		}
		t.Cleanup(func() {
			jsonMarshal = oldMarshal
		})

		tr := &tokenRefresher{fs: testutil.NewMockFileSystem()}
		tokens := &webauthn.AuthTokens{}

		err := tr.saveTokens("/tmp/test/tokens.json", tokens)
		assert.EqualError(t, err, "marshal error")
	})
}

func TestTokenRefresher_Run_InvalidJSON(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.HomeDir = "/home/test"
	mockFS.Files["/home/test/.hourglass-rpa/auth-tokens.json"] = []byte(`invalid json`)

	tr := &tokenRefresher{
		fs: mockFS,
	}

	err := tr.Run()
	assert.Error(t, err)
}

func TestTokenRefresher_Run_HTTPError(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.HomeDir = "/home/test"
	mockFS.Files["/home/test/.hourglass-rpa/auth-tokens.json"] = []byte(`{
		"hg_login": "test-token",
		"xsrf_token": "test-xsrf",
		"expires_at": "2026-03-04T00:00:00Z"
	}`)

	mockHTTP := testutil.NewMockHTTPClient()
	mockHTTP.Err = errors.New("network error")

	tr := &tokenRefresher{
		fs:         mockFS,
		httpClient: mockHTTP,
	}

	err := tr.Run()
	assert.Error(t, err)
}

func TestTokenRefresher_Run_Non200Status(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.HomeDir = "/home/test"
	mockFS.Files["/home/test/.hourglass-rpa/auth-tokens.json"] = []byte(`{
		"hg_login": "test-token",
		"xsrf_token": "test-xsrf",
		"expires_at": "2026-03-04T00:00:00Z"
	}`)

	mockHTTP := testutil.NewMockHTTPClient()
	mockHTTP.Response = testutil.MockErrorResponse(500, "server error")

	tr := &tokenRefresher{
		fs:         mockFS,
		httpClient: mockHTTP,
	}

	err := tr.Run()
	assert.Error(t, err)
}

func TestTokenRefresher_Run_SaveError(t *testing.T) {
	mockFS := testutil.NewMockFileSystem()
	mockFS.HomeDir = "/home/test"
	mockFS.Files["/home/test/.hourglass-rpa/auth-tokens.json"] = []byte(`{
		"hg_login": "test-token",
		"xsrf_token": "test-xsrf",
		"expires_at": "2026-03-04T00:00:00Z"
	}`)
	mockFS.WriteErr = errors.New("write error")

	mockHTTP := testutil.NewMockHTTPClient()
	mockHTTP.Response = &http.Response{
		StatusCode: 200,
		Body:       http.NoBody,
		Header:     http.Header{},
	}

	tr := &tokenRefresher{
		fs:         mockFS,
		httpClient: mockHTTP,
	}

	err := tr.Run()
	assert.Error(t, err)
}

func TestMain(t *testing.T) {
	t.Run("successful run", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		tokensPath := filepath.Join(tempHome, ".hourglass-rpa", "auth-tokens.json")
		err := os.MkdirAll(filepath.Dir(tokensPath), 0700)
		require.NoError(t, err)
		err = os.WriteFile(tokensPath, []byte(`{
			"hg_login": "test-token",
			"xsrf_token": "test-xsrf",
			"expires_at": "2026-03-04T00:00:00Z"
		}`), 0600)
		require.NoError(t, err)

		oldTransport := http.DefaultTransport
		http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     http.Header{},
			}, nil
		})
		t.Cleanup(func() {
			http.DefaultTransport = oldTransport
		})

		oldExit := osExit
		osExit = func(code int) {
			t.Fatalf("osExit should not be called, got code %d", code)
		}
		t.Cleanup(func() {
			osExit = oldExit
		})

		main()
	})

	t.Run("exit on run error", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		oldExit := osExit
		osExit = func(code int) {
			panic(code)
		}
		t.Cleanup(func() {
			osExit = oldExit
		})

		defer func() {
			r := recover()
			require.Equal(t, 1, r)
		}()

		main()
	})
}
