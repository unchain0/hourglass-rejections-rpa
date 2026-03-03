package webauthn

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.do(req)
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPriority_SetCookiesAndGenerateUserID(t *testing.T) {
	a := &Authenticator{}
	a.SetCookies("xsrf-token", "hg-login")
	assert.Equal(t, "xsrf-token", a.xsrfToken)
	assert.Equal(t, "hg-login", a.hgLogin)

	userID := generateUserID()
	decoded, err := base64.StdEncoding.DecodeString(userID)
	require.NoError(t, err)
	assert.Len(t, decoded, 16)
}

func TestPriority_CurlGet(t *testing.T) {
	a := &Authenticator{xsrfToken: "x", hgLogin: "h"}

	t.Run("success", func(t *testing.T) {
		setMockExecCommand(t, "payload\n200", 0)
		body, err := a.curlGet("https://example.com")
		require.NoError(t, err)
		assert.Equal(t, "payload", string(body))
	})

	t.Run("command failure", func(t *testing.T) {
		setMockExecCommand(t, "curl-error", 1)
		_, err := a.curlGet("https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "curl failed")
	})

	t.Run("invalid response", func(t *testing.T) {
		setMockExecCommand(t, "missing-status-code", 0)
		_, err := a.curlGet("https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid curl response")
	})

	t.Run("unexpected status", func(t *testing.T) {
		setMockExecCommand(t, "forbidden\n403", 0)
		_, err := a.curlGet("https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status: 403")
	})
}

func TestPriority_CurlPost(t *testing.T) {
	a := &Authenticator{xsrfToken: "x", hgLogin: "h"}

	t.Run("created status", func(t *testing.T) {
		setMockExecCommand(t, "created\n201", 0)
		body, err := a.curlPost("https://example.com", []byte(`{"ok":true}`))
		require.NoError(t, err)
		assert.Equal(t, "created", string(body))
	})

	t.Run("command failure", func(t *testing.T) {
		setMockExecCommand(t, "post-fail", 1)
		_, err := a.curlPost("https://example.com", []byte("{}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "curl failed")
	})

	t.Run("invalid response", func(t *testing.T) {
		setMockExecCommand(t, "bad", 0)
		_, err := a.curlPost("https://example.com", []byte("{}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid curl response")
	})

	t.Run("unexpected status", func(t *testing.T) {
		setMockExecCommand(t, "fail\n500", 0)
		_, err := a.curlPost("https://example.com", []byte("{}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "registration failed")
	})
}

func TestPriority_LoadCredentialFromPEM(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tempDir := t.TempDir()
		pemPath := filepath.Join(tempDir, "ec.pem")

		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(t, err)

		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
		require.NoError(t, os.WriteFile(pemPath, pemBytes, 0o600))

		cred, err := LoadCredentialFromPEM(pemPath, "cred-id", "hourglass-app.com", "dXNlcg==", "Test")
		require.NoError(t, err)
		require.NotNil(t, cred)
		assert.Equal(t, "cred-id", cred.ID)
		assert.Equal(t, "hourglass-app.com", cred.RPID)

		sig, err := cred.Sign(nil, []byte("12345678901234567890123456789012"))
		require.NoError(t, err)
		assert.NotEmpty(t, sig)
		assert.Equal(t, uint32(1), cred.SignCount)
	})

	t.Run("read error", func(t *testing.T) {
		_, err := LoadCredentialFromPEM("/does/not/exist.pem", "id", "rp", "uid", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read PEM file")
	})

	t.Run("decode error", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "bad.pem")
		require.NoError(t, os.WriteFile(p, []byte("not pem"), 0o600))
		_, err := LoadCredentialFromPEM(p, "id", "rp", "uid", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode PEM block")
	})

	t.Run("parse error", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "parse.pem")
		bad := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")})
		require.NoError(t, os.WriteFile(p, bad, 0o600))
		_, err := LoadCredentialFromPEM(p, "id", "rp", "uid", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse private key")
	})

	t.Run("non ecdsa key", func(t *testing.T) {
		tempDir := t.TempDir()
		p := filepath.Join(tempDir, "rsa.pem")

		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
		require.NoError(t, err)
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
		require.NoError(t, os.WriteFile(p, pemBytes, 0o600))

		_, err = LoadCredentialFromPEM(p, "id", "rp", "uid", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private key is not ECDSA")
	})
}

func TestPriority_CredentialSignError(t *testing.T) {
	cred := &Credential{PrivateKey: []byte("not-a-private-key")}
	_, err := cred.Sign(nil, []byte("123"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get private key")
}

func TestPriority_RawHeadersTransportRoundTrip(t *testing.T) {
	transport := &rawHeadersTransport{
		headers: map[string]string{"X-Test": "new", "X-Trace": "abc"},
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "new", req.Header.Get("X-Test"))
			assert.Equal(t, "abc", req.Header.Get("X-Trace"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)
	req.Header.Set("X-Test", "old")

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
}

func TestPriority_NewClientWithRawHeaders(t *testing.T) {
	client := newClientWithRawHeaders(map[string]string{"X-Token": "abc"})
	require.NotNil(t, client)
	assert.Equal(t, 30*time.Second, client.Timeout)

	rt, ok := client.Transport.(*rawHeadersTransport)
	require.True(t, ok)
	assert.Equal(t, "abc", rt.headers["X-Token"])
	assert.NotNil(t, rt.base)
}

func TestPriority_TokenManagerStartStopRenewalAndGetters(t *testing.T) {
	t.Run("start returns load error", func(t *testing.T) {
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		tokensPath := filepath.Join(tempDir, "tokens.json")
		require.NoError(t, os.WriteFile(tokensPath, []byte("{"), 0o600))

		tm, err := NewTokenManager(storagePath, "https://example.com", WithTokensPath(tokensPath))
		require.NoError(t, err)

		err = tm.Start(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load persisted tokens")
	})

	t.Run("start uses persisted token and stop closes loop", func(t *testing.T) {
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		tokensPath := filepath.Join(tempDir, "tokens.json")

		tokens := &AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: time.Now().Add(2 * time.Hour)}
		data, err := json.Marshal(tokens)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(tokensPath, data, 0o600))

		renewed := make(chan struct{}, 1)
		tm, err := NewTokenManager(storagePath, "https://example.com",
			WithTokensPath(tokensPath),
			WithOnTokenRenewed(func(tokens *AuthTokens) { renewed <- struct{}{} }),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		require.NoError(t, tm.Start(ctx))
		select {
		case <-renewed:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected onTokenRenewed callback")
		}

		assert.Equal(t, "h", tm.GetHGLogin())
		assert.Equal(t, "x", tm.GetXSRFToken())

		tm.Stop()
	})

	t.Run("renewal loop exits on context done", func(t *testing.T) {
		tm := &TokenManager{stopChan: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tm.renewalLoop(ctx)
	})

	t.Run("renewal loop exits on stop channel", func(t *testing.T) {
		tm := &TokenManager{stopChan: make(chan struct{})}
		close(tm.stopChan)
		tm.renewalLoop(context.Background())
	})

	t.Run("getters return empty without tokens", func(t *testing.T) {
		tm := &TokenManager{}
		assert.Equal(t, "", tm.GetHGLogin())
		assert.Equal(t, "", tm.GetXSRFToken())
	})
}

func TestPriority_TokenManagerAuthenticateWithFallback(t *testing.T) {
	tm := &TokenManager{
		authenticator: &Authenticator{storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}},
	}

	_, err := tm.authenticateWithFallback()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "native authentication failed")
}

func TestPriority_BeginAuthenticationErrorPaths(t *testing.T) {
	t.Run("invalid base url", func(t *testing.T) {
		a := &Authenticator{baseURL: "://bad-url", httpClient: defaultHTTPClient}
		_, err := a.beginAuthentication()
		require.Error(t, err)
	})

	t.Run("client do error", func(t *testing.T) {
		a := &Authenticator{
			baseURL: "https://example.com",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network down")
			}},
		}
		_, err := a.beginAuthentication()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network down")
	})

	t.Run("non 200", func(t *testing.T) {
		a := &Authenticator{
			baseURL: "https://example.com",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Body:       io.NopCloser(strings.NewReader("forbidden")),
					Header:     make(http.Header),
				}, nil
			}},
		}
		_, err := a.beginAuthentication()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status: 403")
	})

	t.Run("decode failure", func(t *testing.T) {
		a := &Authenticator{
			baseURL: "https://example.com",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{")),
					Header:     make(http.Header),
				}, nil
			}},
		}
		_, err := a.beginAuthentication()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode response")
	})
}

func TestPriority_AuthenticateErrorPaths(t *testing.T) {
	t.Run("no stored credential", func(t *testing.T) {
		tempDir := t.TempDir()
		auth, err := NewAuthenticator(filepath.Join(tempDir, "credentials.json"), "https://example.com")
		require.NoError(t, err)

		_, err = auth.Authenticate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no stored credential found")
	})

	t.Run("begin authentication fails", func(t *testing.T) {
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		auth, err := NewAuthenticator(storagePath, "https://example.com")
		require.NoError(t, err)

		cred, err := GenerateCredential("hourglass-app.com", "dXNlcg==", "User")
		require.NoError(t, err)
		stored, err := auth.storage.Load()
		require.NoError(t, err)
		stored.Credentials = append(stored.Credentials, *cred)
		require.NoError(t, auth.storage.Save(stored))

		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}}

		_, err = auth.Authenticate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "autenticação automática indisponível")
	})

	t.Run("create assertion fails because invalid private key", func(t *testing.T) {
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		auth, err := NewAuthenticator(storagePath, "https://example.com")
		require.NoError(t, err)

		stored, err := auth.storage.Load()
		require.NoError(t, err)
		stored.Credentials = append(stored.Credentials, Credential{
			ID:         base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef")),
			PrivateKey: []byte("invalid"),
			UserID:     base64.StdEncoding.EncodeToString([]byte("user")),
			RPID:       "hourglass-app.com",
		})
		require.NoError(t, auth.storage.Save(stored))

		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"challenge":"c","timeout":1,"rpId":"hourglass-app.com"}}`)),
				Header:     make(http.Header),
			}, nil
		}}

		_, err = auth.Authenticate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create assertion failed")
	})
}

func TestPriority_HTTPGetAndPostErrors(t *testing.T) {
	t.Run("httpGet request creation error", func(t *testing.T) {
		a := &Authenticator{httpClient: defaultHTTPClient}
		_, err := a.httpGet("://bad-url")
		require.Error(t, err)
	})

	t.Run("httpGet client error", func(t *testing.T) {
		a := &Authenticator{httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("get failed")
		}}}
		_, err := a.httpGet("https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get failed")
	})

	t.Run("httpGet unexpected status", func(t *testing.T) {
		a := &Authenticator{httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("bad gateway")),
				Header:     make(http.Header),
			}, nil
		}}}
		_, err := a.httpGet("https://example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status: 502")
	})

	t.Run("httpPost request creation error", func(t *testing.T) {
		a := &Authenticator{httpClient: defaultHTTPClient}
		_, err := a.httpPost("://bad-url", []byte("{}"))
		require.Error(t, err)
	})

	t.Run("httpPost client error", func(t *testing.T) {
		a := &Authenticator{httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("post failed")
		}}}
		_, err := a.httpPost("https://example.com", []byte("{}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "post failed")
	})

	t.Run("httpPost unexpected status", func(t *testing.T) {
		a := &Authenticator{httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad request")),
				Header:     make(http.Header),
			}, nil
		}}}
		_, err := a.httpPost("https://example.com", []byte("{}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "registration failed: status=400")
	})
}

func TestPriority_FinishAuthenticationErrorPaths(t *testing.T) {
	t.Run("marshal assertion error", func(t *testing.T) {
		a := &Authenticator{}
		assertion := &AssertionResponse{ClientExtensionResults: map[string]interface{}{"bad": make(chan int)}}
		_, err := a.finishAuthentication(assertion)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "marshal assertion")
	})

	t.Run("http client error", func(t *testing.T) {
		a := &Authenticator{
			baseURL: "https://example.com",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network fail")
			}},
		}
		_, err := a.finishAuthentication(&AssertionResponse{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network fail")
	})

	t.Run("non ok response", func(t *testing.T) {
		a := &Authenticator{
			baseURL: "https://example.com",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(strings.NewReader("unauthorized")),
					Header:     make(http.Header),
				}, nil
			}},
		}
		_, err := a.finishAuthentication(&AssertionResponse{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication failed: status=401")
	})

	t.Run("missing cookies", func(t *testing.T) {
		a := &Authenticator{
			baseURL: "https://example.com",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil
			}},
		}
		_, err := a.finishAuthentication(&AssertionResponse{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing authentication cookies")
	})
}

func TestPriority_TokenManagerAuthenticateWithFallbackNativeSuccess(t *testing.T) {
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/webauthn/login/begin":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"publicKey":{"challenge":"c","timeout":60000,"rpId":"hourglass-app.com"}}`))
		case "/auth/webauthn/login/finish":
			http.SetCookie(w, &http.Cookie{Name: "hglogin", Value: "native-hg"})
			http.SetCookie(w, &http.Cookie{Name: "X-Hourglass-XSRF-Token", Value: "native-xsrf"})
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	httpServer := &http.Server{Handler: server}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() {
		_ = httpServer.Shutdown(context.Background())
	})

	baseURL := "http://" + listener.Addr().String()
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	cred, err := GenerateCredential("hourglass-app.com", base64.StdEncoding.EncodeToString([]byte("user")), "User")
	require.NoError(t, err)
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	stored, err := storage.Load()
	require.NoError(t, err)
	stored.Credentials = append(stored.Credentials, *cred)
	require.NoError(t, storage.Save(stored))

	tm, err := NewTokenManager(storagePath, baseURL)
	require.NoError(t, err)

	tokens, err := tm.authenticateWithFallback()
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.Equal(t, "native-hg", tokens.HGLogin)
	assert.Equal(t, "native-xsrf", tokens.XSRFToken)
}

func TestPriority_EnsureValidTokensErrorCallback(t *testing.T) {
	callbackCalled := false
	tm := &TokenManager{
		authenticator:    &Authenticator{storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}},
		renewalThreshold: time.Hour,
		onError: func(err error) {
			callbackCalled = true
		},
	}

	_, err := tm.EnsureValidTokens()
	require.Error(t, err)
	assert.True(t, callbackCalled)
}

func TestPriority_BrowserFallbackErrorPathAndChromePath(t *testing.T) {
	t.Run("getChromePath prefers CHROME_BIN", func(t *testing.T) {
		t.Setenv("CHROME_BIN", "/tmp/chrome-bin")
		t.Setenv("CHROME_PATH", "/tmp/chrome-path")
		assert.Equal(t, "/tmp/chrome-bin", getChromePath())
	})

	t.Run("getChromePath uses CHROME_PATH when bin empty", func(t *testing.T) {
		t.Setenv("CHROME_BIN", "")
		t.Setenv("CHROME_PATH", "/tmp/chrome-path")
		assert.Equal(t, "/tmp/chrome-path", getChromePath())
	})

	t.Run("authenticateWithFallback wraps browser fallback error", func(t *testing.T) {
		t.Setenv("CHROME_BIN", "/definitely/not/a/real/chrome")
		t.Setenv("CHROME_PATH", "")

		tm := &TokenManager{
			authenticator: &Authenticator{storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}},
			browserAuth:   NewBrowserAuth("https://example.com"),
		}

		_, err := tm.authenticateWithFallback()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "browser auth fallback failed")
	})
}

func TestPriority_FinishRegistrationErrorPath(t *testing.T) {
	a := &Authenticator{
		baseURL: "https://example.com",
		httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		}},
	}

	err := a.finishRegistration(&AttestationResponse{Type: "public-key"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registration failed")
}

func TestPriority_NewTokenManagerUsesEnvPaths(t *testing.T) {
	tempDir := t.TempDir()
	credentialsPath := filepath.Join(tempDir, "from-env-credentials.json")
	tokensPath := filepath.Join(tempDir, "from-env-tokens.json")

	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", credentialsPath)
	t.Setenv("WEBAUTHN_TOKENS_PATH", tokensPath)

	tm, err := NewTokenManager("", "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, credentialsPath, tm.storagePath)
	assert.Equal(t, tokensPath, tm.tokensPath)
}

func TestPriority_WithTokensPathIgnoresEmpty(t *testing.T) {
	tm := &TokenManager{tokensPath: "keep-me"}
	WithTokensPath("")(tm)
	assert.Equal(t, "keep-me", tm.tokensPath)
}

func setMockExecCommand(t *testing.T, output string, exitCode int) {
	t.Helper()
	original := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cmdArgs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
		cmd := exec.Command(os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_OUTPUT_B64="+base64.StdEncoding.EncodeToString([]byte(output)),
			"HELPER_EXIT_CODE="+strconv.Itoa(exitCode),
		)
		return cmd
	}
	t.Cleanup(func() {
		execCommand = original
	})
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	encoded := os.Getenv("HELPER_OUTPUT_B64")
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	_, _ = fmt.Fprint(os.Stdout, string(decoded))

	exitCode, _ := strconv.Atoi(os.Getenv("HELPER_EXIT_CODE"))
	os.Exit(exitCode)
}
