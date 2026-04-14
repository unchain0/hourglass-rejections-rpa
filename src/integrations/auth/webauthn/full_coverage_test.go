package webauthn

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreWebAuthnCoreHooks(t *testing.T) {
	t.Helper()

	originalOsMkdirAllTypes := osMkdirAllTypes
	originalOsReadFileTypes := osReadFileTypes
	originalOsWriteFileTypes := osWriteFileTypes
	originalJSONMarshalIndentTypes := jsonMarshalIndentTypes
	originalECDSAGenerateKey := ecdsaGenerateKey
	originalX509MarshalPKCS8PrivateKey := x509MarshalPKCS8PrivateKey
	originalIOReadFullTypes := ioReadFullTypes
	originalCBORMarshalTypes := cborMarshalTypes
	originalX509ParsePKCS8PrivateKey := x509ParsePKCS8PrivateKey
	originalECDSASignASN1Types := ecdsaSignASN1Types
	originalJSONMarshalAuthentication := jsonMarshalAuthentication
	originalCreateAssertionAuthData := createAssertionAuthData
	originalCreateAssertionSignature := createAssertionSignature
	originalECDSASignAuthentication := ecdsaSignAuthentication
	originalGenerateCredentialAuthenticator := generateCredentialAuthenticator
	originalCreateAttestationAuthenticator := createAttestationAuthenticator
	originalFinishRegistrationAuthenticator := finishRegistrationAuthenticator
	originalCreateAuthenticatorDataAuthenticator := createAuthenticatorDataAuthenticator
	originalCreateAttestationObjectAuthenticator := createAttestationObjectAuthenticator
	originalJSONMarshalAuthenticator := jsonMarshalAuthenticator
	originalOSUserHomeDirTokenManager := osUserHomeDirTokenManager
	originalOSMkdirAllTokenManager := osMkdirAllTokenManager
	originalOSWriteFileTokenManager := osWriteFileTokenManager
	originalOSRenameTokenManager := osRenameTokenManager
	originalOSRemoveTokenManager := osRemoveTokenManager
	originalOSStatTokenManager := osStatTokenManager
	originalOSReadFileTokenManager := osReadFileTokenManager
	originalJSONMarshalTokenManager := jsonMarshalTokenManager
	originalJSONUnmarshalTokenManager := jsonUnmarshalTokenManager
	originalNewRenewalTicker := newRenewalTicker

	t.Cleanup(func() {
		osMkdirAllTypes = originalOsMkdirAllTypes
		osReadFileTypes = originalOsReadFileTypes
		osWriteFileTypes = originalOsWriteFileTypes
		jsonMarshalIndentTypes = originalJSONMarshalIndentTypes
		ecdsaGenerateKey = originalECDSAGenerateKey
		x509MarshalPKCS8PrivateKey = originalX509MarshalPKCS8PrivateKey
		ioReadFullTypes = originalIOReadFullTypes
		cborMarshalTypes = originalCBORMarshalTypes
		x509ParsePKCS8PrivateKey = originalX509ParsePKCS8PrivateKey
		ecdsaSignASN1Types = originalECDSASignASN1Types
		jsonMarshalAuthentication = originalJSONMarshalAuthentication
		createAssertionAuthData = originalCreateAssertionAuthData
		createAssertionSignature = originalCreateAssertionSignature
		ecdsaSignAuthentication = originalECDSASignAuthentication
		generateCredentialAuthenticator = originalGenerateCredentialAuthenticator
		createAttestationAuthenticator = originalCreateAttestationAuthenticator
		finishRegistrationAuthenticator = originalFinishRegistrationAuthenticator
		createAuthenticatorDataAuthenticator = originalCreateAuthenticatorDataAuthenticator
		createAttestationObjectAuthenticator = originalCreateAttestationObjectAuthenticator
		jsonMarshalAuthenticator = originalJSONMarshalAuthenticator
		osUserHomeDirTokenManager = originalOSUserHomeDirTokenManager
		osMkdirAllTokenManager = originalOSMkdirAllTokenManager
		osWriteFileTokenManager = originalOSWriteFileTokenManager
		osRenameTokenManager = originalOSRenameTokenManager
		osRemoveTokenManager = originalOSRemoveTokenManager
		osStatTokenManager = originalOSStatTokenManager
		osReadFileTokenManager = originalOSReadFileTokenManager
		jsonMarshalTokenManager = originalJSONMarshalTokenManager
		jsonUnmarshalTokenManager = originalJSONUnmarshalTokenManager
		newRenewalTicker = originalNewRenewalTicker
	})
}

func mustWriteECDSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "credential.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0o600))
	return path
}

func mustGenerateCredentialForTest(t *testing.T) *Credential {
	t.Helper()

	cred, err := GenerateCredential("hourglass-app.com", base64.StdEncoding.EncodeToString([]byte("user")), "Test User")
	require.NoError(t, err)
	return cred
}

func mustStoreCredential(t *testing.T, storagePath string, cred *Credential) *Storage {
	t.Helper()

	storage, err := NewStorage(storagePath)
	require.NoError(t, err)

	stored, err := storage.Load()
	require.NoError(t, err)
	stored.Credentials = append(stored.Credentials, *cred)
	require.NoError(t, storage.Save(stored))

	return storage
}

type fakeTicker struct {
	ch chan time.Time
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTicker) Stop() {}

func TestCoverageBrowserAuthDefaultPrompt(t *testing.T) {
	restoreBrowserAuthHooks(t)

	runCalls := 0
	chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
		runCalls++
		return nil
	}

	var clicked bool
	err := triggerWebAuthnPrompt(context.Background(), &clicked)
	require.NoError(t, err)
	assert.False(t, clicked)
	assert.Equal(t, 1, runCalls)
}

func TestCoverageEnvironmentBranches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("headless detection branches depend on non-Windows environment")
	}

	t.Run("display present is not headless", func(t *testing.T) {
		t.Setenv("DISPLAY", ":0")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("CI", "")
		assert.False(t, IsHeadlessEnvironment())
	})

	t.Run("ssh without display is headless", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "1")
		t.Setenv("SSH_CLIENT", "")
		assert.True(t, IsHeadlessEnvironment())
	})

	t.Run("ci without display is headless", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("CI", "true")
		assert.True(t, IsHeadlessEnvironment())
	})

	t.Run("no display defaults to headless", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("CI", "")
		t.Setenv("GITHUB_ACTIONS", "")
		assert.True(t, IsHeadlessEnvironment())
	})

	t.Run("has credentials handles edge cases", func(t *testing.T) {
		assert.False(t, (&TokenManager{}).HasWebAuthnCredentials())

		tempFile := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(tempFile, []byte("x"), 0o600))
		assert.False(t, (&TokenManager{storagePath: filepath.Join(tempFile, "credentials.json")}).HasWebAuthnCredentials())

		assert.False(t, (&TokenManager{storagePath: t.TempDir()}).HasWebAuthnCredentials())

		storagePath := filepath.Join(t.TempDir(), "credentials.json")
		cred := mustGenerateCredentialForTest(t)
		mustStoreCredential(t, storagePath, cred)
		assert.True(t, (&TokenManager{storagePath: storagePath}).HasWebAuthnCredentials())
	})
}

func TestCoverageTypesBranches(t *testing.T) {
	t.Run("new storage returns mkdir error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		osMkdirAllTypes = func(string, os.FileMode) error { return errors.New("mkdir failed") }

		_, err := NewStorage("/tmp/credentials.json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create credentials directory")
	})

	t.Run("load returns read error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		osReadFileTypes = func(string) ([]byte, error) { return nil, errors.New("read failed") }

		_, err := (&Storage{path: "/tmp/credentials.json"}).Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read credentials file")
	})

	t.Run("save returns marshal error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		jsonMarshalIndentTypes = func(v any, prefix, indent string) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}

		err := (&Storage{path: "/tmp/credentials.json"}).Save(&StoredCredentials{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal credentials")
	})

	t.Run("generate credential covers all error branches", func(t *testing.T) {
		t.Run("key generation error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			ecdsaGenerateKey = func(elliptic.Curve, io.Reader) (*ecdsa.PrivateKey, error) {
				return nil, errors.New("keygen failed")
			}

			_, err := GenerateCredential("rp", "user", "name")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to generate key pair")
		})

		t.Run("marshal private key error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			x509MarshalPKCS8PrivateKey = func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			}

			_, err := GenerateCredential("rp", "user", "name")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to marshal private key")
		})

		t.Run("credential id generation error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			ioReadFullTypes = func(io.Reader, []byte) (int, error) {
				return 0, errors.New("random read failed")
			}

			_, err := GenerateCredential("rp", "user", "name")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to generate credential ID")
		})

		t.Run("cose public key error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cborMarshalTypes = func(any) ([]byte, error) {
				return nil, errors.New("cbor failed")
			}

			_, err := GenerateCredential("rp", "user", "name")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to create COSE public key")
		})
	})

	t.Run("load credential from pem covers remaining errors", func(t *testing.T) {
		t.Run("marshal private key error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			x509MarshalPKCS8PrivateKey = func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			}

			_, err := LoadCredentialFromPEM(mustWriteECDSAPrivateKeyPEM(t), "id", "rp", "uid", "name")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to marshal private key")
		})

		t.Run("cose public key error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cborMarshalTypes = func(any) ([]byte, error) {
				return nil, errors.New("cbor failed")
			}

			_, err := LoadCredentialFromPEM(mustWriteECDSAPrivateKeyPEM(t), "id", "rp", "uid", "name")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to create COSE public key")
		})
	})

	t.Run("get private key rejects non ecdsa keys", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		rsaKey, err := rsa.GenerateKey(crand.Reader, 2048)
		require.NoError(t, err)

		pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
		require.NoError(t, err)

		_, err = (&Credential{PrivateKey: pkcs8}).GetPrivateKey()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private key is not ECDSA")
	})

	t.Run("sign returns signing error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		cred := mustGenerateCredentialForTest(t)
		ecdsaSignASN1Types = func(io.Reader, *ecdsa.PrivateKey, []byte) ([]byte, error) {
			return nil, errors.New("sign failed")
		}

		_, err := cred.Sign(nil, []byte("12345678901234567890123456789012"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sign:")
	})
}

func TestCoverageAuthenticationBranches(t *testing.T) {
	t.Run("stored credential helpers handle errors", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		auth := &Authenticator{storage: &Storage{path: t.TempDir()}}

		_, err := auth.getStoredCredential()
		require.Error(t, err)

		err = auth.updateStoredCredential(&Credential{ID: "missing"})
		require.Error(t, err)

		storagePath := filepath.Join(t.TempDir(), "credentials.json")
		storage := mustStoreCredential(t, storagePath, mustGenerateCredentialForTest(t))
		auth.storage = storage

		err = auth.updateStoredCredential(&Credential{ID: "missing"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credential not found")
	})

	t.Run("begin authentication forwards cookie and stores response cookies", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		auth := &Authenticator{
			baseURL: "https://example.com",
			hgLogin: "existing-hg",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				cookie, err := req.Cookie("hglogin")
				require.NoError(t, err)
				assert.Equal(t, "existing-hg", cookie.Value)

				header := make(http.Header)
				header.Add("Set-Cookie", "hglogin=refreshed-hg")
				header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=refreshed-xsrf")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"publicKey":{"challenge":"challenge","timeout":1,"rpId":"hourglass-app.com"}}`,
					)),
					Header: header,
				}, nil
			}},
		}

		resp, err := auth.beginAuthentication()
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "refreshed-hg", auth.hgLogin)
		assert.Equal(t, "refreshed-xsrf", auth.xsrfToken)
	})

	t.Run("create assertion covers remaining error branches", func(t *testing.T) {
		t.Run("marshal client data error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cred := mustGenerateCredentialForTest(t)
			jsonMarshalAuthentication = func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			}

			_, err := (&Authenticator{}).createAssertion(cred, &BeginAuthenticationResponse{
				PublicKey: struct {
					Challenge string `json:"challenge"`
					Timeout   int64  `json:"timeout"`
					RpID      string `json:"rpId"`
				}{Challenge: "c", RpID: "hourglass-app.com"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "marshal client data")
		})

		t.Run("authenticator data error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cred := mustGenerateCredentialForTest(t)
			createAssertionAuthData = func(*Authenticator, *Credential, []byte) ([]byte, error) {
				return nil, errors.New("auth data failed")
			}

			_, err := (&Authenticator{}).createAssertion(cred, &BeginAuthenticationResponse{
				PublicKey: struct {
					Challenge string `json:"challenge"`
					Timeout   int64  `json:"timeout"`
					RpID      string `json:"rpId"`
				}{Challenge: "c", RpID: "hourglass-app.com"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "create authenticator data")
		})

		t.Run("signature error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cred := mustGenerateCredentialForTest(t)
			createAssertionSignature = func(*Authenticator, *ecdsa.PrivateKey, []byte, []byte) ([]byte, error) {
				return nil, errors.New("signature failed")
			}

			_, err := (&Authenticator{}).createAssertion(cred, &BeginAuthenticationResponse{
				PublicKey: struct {
					Challenge string `json:"challenge"`
					Timeout   int64  `json:"timeout"`
					RpID      string `json:"rpId"`
				}{Challenge: "c", RpID: "hourglass-app.com"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "create signature")
		})
	})

	t.Run("create signature returns signing error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
		require.NoError(t, err)

		ecdsaSignAuthentication = func(io.Reader, *ecdsa.PrivateKey, []byte) (*big.Int, *big.Int, error) {
			return nil, nil, errors.New("sign failed")
		}

		_, err = (&Authenticator{}).createSignature(key, []byte("auth"), []byte("client"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sign failed")
	})

	t.Run("finish authentication returns request creation error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		_, err := (&Authenticator{baseURL: "://bad-url"}).finishAuthentication(&AssertionResponse{})
		require.Error(t, err)
	})

	t.Run("authenticate wraps finish and update errors", func(t *testing.T) {
		t.Run("finish authentication error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			storagePath := filepath.Join(t.TempDir(), "credentials.json")
			cred := mustGenerateCredentialForTest(t)
			storage := mustStoreCredential(t, storagePath, cred)

			auth := &Authenticator{
				storage: storage,
				baseURL: "https://example.com",
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/auth/webauthn/login/begin":
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: io.NopCloser(strings.NewReader(
								`{"publicKey":{"challenge":"challenge","timeout":1,"rpId":"hourglass-app.com"}}`,
							)),
							Header: make(http.Header),
						}, nil
					case "/auth/webauthn/login/finish":
						return &http.Response{
							StatusCode: http.StatusUnauthorized,
							Body:       io.NopCloser(strings.NewReader("unauthorized")),
							Header:     make(http.Header),
						}, nil
					default:
						t.Fatalf("unexpected request to %s", req.URL.Path)
						return nil, nil
					}
				}},
			}

			_, err := auth.Authenticate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "finish authentication failed")
		})

		t.Run("update stored credential error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			storagePath := filepath.Join(t.TempDir(), "credentials.json")
			cred := mustGenerateCredentialForTest(t)
			storage := mustStoreCredential(t, storagePath, cred)

			auth := &Authenticator{
				storage: storage,
				baseURL: "https://example.com",
			}
			badPath := t.TempDir()
			auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/auth/webauthn/login/begin":
					header := make(http.Header)
					header.Add("Set-Cookie", "hglogin=begin-hg")
					header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=begin-xsrf")
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(
							`{"publicKey":{"challenge":"challenge","timeout":1,"rpId":"hourglass-app.com"}}`,
						)),
						Header: header,
					}, nil
				case "/auth/webauthn/login/finish":
					auth.storage.path = badPath
					header := make(http.Header)
					header.Add("Set-Cookie", "hglogin=final-hg")
					header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=final-xsrf")
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("ok")),
						Header:     header,
					}, nil
				default:
					t.Fatalf("unexpected request to %s", req.URL.Path)
					return nil, nil
				}
			}}

			_, err := auth.Authenticate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "update credential failed")
		})
	})
}

func TestCoverageAuthenticatorBranches(t *testing.T) {
	t.Run("new authenticator returns storage error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		tempFile := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(tempFile, []byte("x"), 0o600))

		_, err := NewAuthenticator(filepath.Join(tempFile, "credentials.json"), "https://example.com")
		require.Error(t, err)
	})

	t.Run("register covers remaining branches", func(t *testing.T) {
		t.Run("defaults empty user id and rp id", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			storagePath := filepath.Join(t.TempDir(), "credentials.json")
			auth, err := NewAuthenticator(storagePath, "https://example.com")
			require.NoError(t, err)
			auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/auth/webauthn/register/begin":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(
							`{"publicKey":{"rp":{"name":"Hourglass","id":""},"user":{"name":"Test","displayName":"Test","id":""},"challenge":"challenge"}}`,
						)),
						Header: make(http.Header),
					}, nil
				case "/auth/webauthn/register/finish":
					return &http.Response{
						StatusCode: http.StatusCreated,
						Body:       io.NopCloser(strings.NewReader("created")),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected request to %s", req.URL.Path)
					return nil, nil
				}
			}}

			cred, err := auth.Register("Test User")
			require.NoError(t, err)
			assert.NotEmpty(t, cred.UserID)
			assert.Equal(t, "hourglass-app.com", cred.RPID)
		})

		t.Run("begin registration error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			auth := &Authenticator{
				baseURL: "https://example.com",
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("begin failed")
				}},
				storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")},
			}

			_, err := auth.Register("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "begin registration failed")
		})

		t.Run("generate credential error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			generateCredentialAuthenticator = func(string, string, string) (*Credential, error) {
				return nil, errors.New("generate failed")
			}

			auth := &Authenticator{
				baseURL: "https://example.com",
				storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")},
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"rp":{"id":"hourglass-app.com"},"user":{"id":"user"},"challenge":"challenge"}}`)),
						Header:     make(http.Header),
					}, nil
				}},
			}

			_, err := auth.Register("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "generate credential failed")
		})

		t.Run("create attestation error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			createAttestationAuthenticator = func(*Authenticator, *Credential, *BeginRegistrationResponse) (*AttestationResponse, error) {
				return nil, errors.New("attestation failed")
			}

			auth := &Authenticator{
				baseURL: "https://example.com",
				storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")},
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"rp":{"id":"hourglass-app.com"},"user":{"id":"user"},"challenge":"challenge"}}`)),
						Header:     make(http.Header),
					}, nil
				}},
			}

			_, err := auth.Register("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "create attestation failed")
		})

		t.Run("finish registration error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			finishRegistrationAuthenticator = func(*Authenticator, *AttestationResponse) error {
				return errors.New("finish failed")
			}

			auth := &Authenticator{
				baseURL: "https://example.com",
				storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")},
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"rp":{"id":"hourglass-app.com"},"user":{"id":"user"},"challenge":"challenge"}}`)),
						Header:     make(http.Header),
					}, nil
				}},
			}

			_, err := auth.Register("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "finish registration failed")
		})

		t.Run("load credentials error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			auth := &Authenticator{
				baseURL: "https://example.com",
				storage: &Storage{path: t.TempDir()},
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"rp":{"id":"hourglass-app.com"},"user":{"id":"user"},"challenge":"challenge"}}`)),
						Header:     make(http.Header),
					}, nil
				}},
			}
			generateCredentialAuthenticator = func(string, string, string) (*Credential, error) {
				return &Credential{ID: base64.RawURLEncoding.EncodeToString([]byte("cred-identifier")), UserID: "user", RPID: "hourglass-app.com"}, nil
			}
			createAttestationAuthenticator = func(*Authenticator, *Credential, *BeginRegistrationResponse) (*AttestationResponse, error) {
				return &AttestationResponse{Type: "public-key"}, nil
			}
			finishRegistrationAuthenticator = func(*Authenticator, *AttestationResponse) error { return nil }

			_, err := auth.Register("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "load credentials failed")
		})

		t.Run("save credential error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osWriteFileTypes = func(string, []byte, os.FileMode) error {
				return errors.New("write failed")
			}

			storagePath := filepath.Join(t.TempDir(), "credentials.json")
			auth, err := NewAuthenticator(storagePath, "https://example.com")
			require.NoError(t, err)
			auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"rp":{"id":"hourglass-app.com"},"user":{"id":"user"},"challenge":"challenge"}}`)),
					Header:     make(http.Header),
				}, nil
			}}
			createAttestationAuthenticator = func(*Authenticator, *Credential, *BeginRegistrationResponse) (*AttestationResponse, error) {
				return &AttestationResponse{Type: "public-key"}, nil
			}
			finishRegistrationAuthenticator = func(*Authenticator, *AttestationResponse) error { return nil }

			_, err = auth.Register("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "save credential failed")
		})
	})

	t.Run("begin registration covers curl and decode branches", func(t *testing.T) {
		t.Run("uses curl when cookies are present", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			setMockExecCommand(t, `{"publicKey":{"rp":{"id":"hourglass-app.com"},"user":{"id":"user"},"challenge":"challenge"}}`+"\n200", 0)

			auth := &Authenticator{baseURL: "https://example.com", xsrfToken: "x", hgLogin: "h"}
			resp, err := auth.beginRegistration("Test User")
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, "hourglass-app.com", resp.PublicKey.Rp.ID)
		})

		t.Run("decode error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			auth := &Authenticator{
				baseURL: "https://example.com",
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("{")),
						Header:     make(http.Header),
					}, nil
				}},
			}

			_, err := auth.beginRegistration("Test User")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "decode response")
		})
	})

	t.Run("create attestation covers remaining error branches", func(t *testing.T) {
		t.Run("marshal client data error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cred := mustGenerateCredentialForTest(t)
			jsonMarshalAuthenticator = func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			}

			_, err := (&Authenticator{}).createAttestation(cred, &BeginRegistrationResponse{
				PublicKey: PublicKeyClass{Rp: Rp{ID: "hourglass-app.com"}, Challenge: "challenge"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "marshal client data")
		})

		t.Run("authenticator data error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cred := mustGenerateCredentialForTest(t)
			createAuthenticatorDataAuthenticator = func(*Authenticator, *Credential, []byte) ([]byte, error) {
				return nil, errors.New("auth data failed")
			}

			_, err := (&Authenticator{}).createAttestation(cred, &BeginRegistrationResponse{
				PublicKey: PublicKeyClass{Rp: Rp{ID: "hourglass-app.com"}, Challenge: "challenge"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "create authenticator data")
		})

		t.Run("attestation object error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			cred := mustGenerateCredentialForTest(t)
			createAttestationObjectAuthenticator = func(*Authenticator, []byte) ([]byte, error) {
				return nil, errors.New("attestation object failed")
			}

			_, err := (&Authenticator{}).createAttestation(cred, &BeginRegistrationResponse{
				PublicKey: PublicKeyClass{Rp: Rp{ID: "hourglass-app.com"}, Challenge: "challenge"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "create attestation object")
		})
	})

	t.Run("create authenticator data rejects invalid credential ids", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		cred := &Credential{ID: "!!!", PublicKey: []byte("pub"), RPID: "hourglass-app.com"}

		_, err := (&Authenticator{}).createAuthenticatorData(cred, []byte("client"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get credential ID bytes")
	})

	t.Run("debug auth data covers short branches", func(t *testing.T) {
		debugAuthData([]byte("short"))

		authData := make([]byte, 54)
		authData[32] = 0x40
		debugAuthData(authData)
	})

	t.Run("finish registration covers marshal, curl, and empty-body success", func(t *testing.T) {
		t.Run("marshal error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			jsonMarshalAuthenticator = func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			}

			err := (&Authenticator{baseURL: "https://example.com"}).finishRegistration(&AttestationResponse{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "marshal attestation")
		})

		t.Run("uses curl path", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			setMockExecCommand(t, "created\n201", 0)

			err := (&Authenticator{
				baseURL:   "https://example.com",
				xsrfToken: "x",
				hgLogin:   "h",
			}).finishRegistration(&AttestationResponse{Type: "public-key"})
			require.NoError(t, err)
		})

		t.Run("returns nil on empty http body", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			auth := &Authenticator{
				baseURL: "https://example.com",
				httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusCreated,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}},
			}

			require.NoError(t, auth.finishRegistration(&AttestationResponse{Type: "public-key"}))
		})
	})
}

func TestCoverageTokenManagerBranches(t *testing.T) {
	t.Run("new token manager covers home dir, headless, and authenticator errors", func(t *testing.T) {
		t.Run("uses home dir fallback and disables browser auth when headless", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			tempDir := t.TempDir()
			t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
			t.Setenv("WEBAUTHN_TOKENS_PATH", "")
			t.Setenv("DISPLAY", "")
			t.Setenv("CI", "true")
			osUserHomeDirTokenManager = func() (string, error) { return tempDir, nil }

			tm, err := NewTokenManager("", "https://example.com")
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(tempDir, ".hourglass-rpa", "webauthn-credentials.json"), tm.storagePath)
			assert.Equal(t, filepath.Join(tempDir, ".hourglass-rpa", "auth-tokens.json"), tm.tokensPath)
			assert.Nil(t, tm.browserAuth)
		})

		t.Run("propagates authenticator construction error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			tempFile := filepath.Join(t.TempDir(), "not-a-dir")
			require.NoError(t, os.WriteFile(tempFile, []byte("x"), 0o600))

			_, err := NewTokenManager(filepath.Join(tempFile, "credentials.json"), "https://example.com")
			require.Error(t, err)
		})
	})

	t.Run("start returns ensure valid tokens error", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		tm, err := NewTokenManager(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com", WithBrowserAuth(nil))
		require.NoError(t, err)

		err = tm.Start(context.Background())
		require.Error(t, err)
	})

	t.Run("ensure valid tokens covers concurrent refresh and incomplete tokens", func(t *testing.T) {
		t.Run("returns tokens refreshed by another goroutine", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			tm := &TokenManager{renewalThreshold: time.Hour, stopChan: make(chan struct{})}
			tm.setTokens(&AuthTokens{HGLogin: "old", XSRFToken: "old", ExpiresAt: time.Now().Add(10 * time.Minute)})

			tm.renewMu.Lock()
			done := make(chan struct{})
			var (
				got *AuthTokens
				err error
			)

			go func() {
				got, err = tm.EnsureValidTokens()
				close(done)
			}()

			time.Sleep(10 * time.Millisecond)
			fresh := &AuthTokens{HGLogin: "new", XSRFToken: "new", ExpiresAt: time.Now().Add(2 * time.Hour)}
			tm.setTokens(fresh)
			tm.renewMu.Unlock()

			<-done
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "new", got.HGLogin)
		})

		t.Run("reauthenticates incomplete tokens and ignores save failure", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)

			storagePath := filepath.Join(t.TempDir(), "credentials.json")
			cred := mustGenerateCredentialForTest(t)
			storage := mustStoreCredential(t, storagePath, cred)

			tm := &TokenManager{
				authenticator:    &Authenticator{storage: storage, baseURL: "https://example.com"},
				storagePath:      storagePath,
				tokensPath:       "",
				renewalThreshold: time.Hour,
				stopChan:         make(chan struct{}),
			}
			tm.authenticator.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/auth/webauthn/login/begin":
					header := make(http.Header)
					header.Add("Set-Cookie", "hglogin=begin-hg")
					header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=begin-xsrf")
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"challenge":"challenge","timeout":1,"rpId":"hourglass-app.com"}}`)),
						Header:     header,
					}, nil
				case "/auth/webauthn/login/finish":
					header := make(http.Header)
					header.Add("Set-Cookie", "hglogin=final-hg")
					header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=final-xsrf")
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("ok")),
						Header:     header,
					}, nil
				default:
					t.Fatalf("unexpected request to %s", req.URL.Path)
					return nil, nil
				}
			}}
			tm.setTokens(&AuthTokens{HGLogin: "incomplete", ExpiresAt: time.Now().Add(2 * time.Hour)})

			tokens, err := tm.EnsureValidTokens()
			require.NoError(t, err)
			require.NotNil(t, tokens)
			assert.Equal(t, "final-hg", tokens.HGLogin)
			assert.Equal(t, "final-xsrf", tokens.XSRFToken)
		})
	})

	t.Run("tokens need renewal and base url normalization cover remaining branches", func(t *testing.T) {
		tm := &TokenManager{renewalThreshold: time.Hour}
		assert.True(t, tm.tokensNeedRenewal(nil))
		assert.True(t, tm.tokensNeedRenewal(&AuthTokens{HGLogin: "only-one"}))
		assert.True(t, tm.tokensNeedRenewal(&AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: time.Now().Add(10 * time.Minute)}))
		assert.False(t, tm.tokensNeedRenewal(&AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: time.Now().Add(2 * time.Hour)}))

		assert.Equal(t, "", normalizeWebAuthnBaseURL(""))
		assert.Equal(t, "://bad-url", normalizeWebAuthnBaseURL("://bad-url"))
	})

	t.Run("save tokens covers remaining error branches", func(t *testing.T) {
		tokens := &AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: time.Now().Add(time.Hour)}

		t.Run("marshal error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			jsonMarshalTokenManager = func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			}

			err := (&TokenManager{tokensPath: "/tmp/tokens.json"}).SaveTokens(tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to marshal tokens")
		})

		t.Run("mkdir error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osMkdirAllTokenManager = func(string, os.FileMode) error {
				return errors.New("mkdir failed")
			}

			err := (&TokenManager{tokensPath: "/tmp/tokens.json"}).SaveTokens(tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to create tokens directory")
		})

		t.Run("write temp file error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osWriteFileTokenManager = func(string, []byte, os.FileMode) error {
				return errors.New("write failed")
			}

			err := (&TokenManager{tokensPath: "/tmp/tokens.json"}).SaveTokens(tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to write temp tokens file")
		})

		t.Run("rename error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osWriteFileTokenManager = func(string, []byte, os.FileMode) error { return nil }
			osRenameTokenManager = func(string, string) error { return errors.New("rename failed") }

			err := (&TokenManager{tokensPath: "/tmp/tokens.json"}).SaveTokens(tokens)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to rename tokens file")
		})

		t.Run("stat error is ignored", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osWriteFileTokenManager = func(string, []byte, os.FileMode) error { return nil }
			osRenameTokenManager = func(string, string) error { return nil }
			osStatTokenManager = func(string) (os.FileInfo, error) { return nil, errors.New("stat failed") }

			require.NoError(t, (&TokenManager{tokensPath: "/tmp/tokens.json"}).SaveTokens(tokens))
		})
	})

	t.Run("load tokens covers remaining error branches", func(t *testing.T) {
		t.Run("empty path", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			_, err := (&TokenManager{}).LoadTokens()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tokens path is not configured")
		})

		t.Run("read error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osReadFileTokenManager = func(string) ([]byte, error) {
				return nil, errors.New("read failed")
			}

			_, err := (&TokenManager{tokensPath: "/tmp/tokens.json"}).LoadTokens()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to read tokens file")
		})

		t.Run("unmarshal error", func(t *testing.T) {
			restoreWebAuthnCoreHooks(t)
			osReadFileTokenManager = func(string) ([]byte, error) {
				return []byte(`{"bad":true}`), nil
			}
			jsonUnmarshalTokenManager = func([]byte, any) error {
				return errors.New("unmarshal failed")
			}

			_, err := (&TokenManager{tokensPath: "/tmp/tokens.json"}).LoadTokens()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to unmarshal tokens file")
		})
	})

	t.Run("authenticate with fallback returns browser auth success", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)
		restoreBrowserAuthHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")

		chromedpRun = func(context.Context, ...chromedp.Action) error { return nil }
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{HasAuthButton: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			*clicked = true
			return nil
		}
		getCookies = func(context.Context) ([]*network.Cookie, error) {
			return browserAuthCookies(), nil
		}

		tm := &TokenManager{
			browserAuth: NewBrowserAuth("https://example.com"),
			stopChan:    make(chan struct{}),
		}

		tokens, err := tm.authenticateWithFallback()
		require.NoError(t, err)
		require.NotNil(t, tokens)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)
	})

	t.Run("renewal loop handles ticker errors", func(t *testing.T) {
		restoreWebAuthnCoreHooks(t)

		ticker := &fakeTicker{ch: make(chan time.Time, 1)}
		newRenewalTicker = func(time.Duration) renewalTicker { return ticker }

		tm := &TokenManager{
			stopChan:         make(chan struct{}),
			renewalThreshold: time.Hour,
		}

		done := make(chan struct{})
		go func() {
			tm.renewalLoop(context.Background())
			close(done)
		}()

		ticker.ch <- time.Now()
		time.Sleep(10 * time.Millisecond)
		close(tm.stopChan)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("renewalLoop did not stop")
		}
	})
}
