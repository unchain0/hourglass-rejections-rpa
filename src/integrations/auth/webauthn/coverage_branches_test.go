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
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRenewalTicker struct {
	ch chan time.Time
}

func (f *fakeRenewalTicker) C() <-chan time.Time {
	return f.ch
}

func (f *fakeRenewalTicker) Stop() {}

var (
	defaultOSMkdirAllTypes             = osMkdirAllTypes
	defaultOSReadFileTypes             = osReadFileTypes
	defaultOSWriteFileTypes            = osWriteFileTypes
	defaultJSONMarshalIndentTypes      = jsonMarshalIndentTypes
	defaultECDSAGenerateKey            = ecdsaGenerateKey
	defaultX509MarshalPKCS8PrivateKey  = x509MarshalPKCS8PrivateKey
	defaultIOReadFullTypes             = ioReadFullTypes
	defaultCBORMarshalTypes            = cborMarshalTypes
	defaultX509ParsePKCS8PrivateKey    = x509ParsePKCS8PrivateKey
	defaultECDSASignASN1Types          = ecdsaSignASN1Types
	defaultOSUserHomeDirTokenManager   = osUserHomeDirTokenManager
	defaultOSMkdirAllTokenManager      = osMkdirAllTokenManager
	defaultOSWriteFileTokenManager     = osWriteFileTokenManager
	defaultOSRenameTokenManager        = osRenameTokenManager
	defaultOSRemoveTokenManager        = osRemoveTokenManager
	defaultOSStatTokenManager          = osStatTokenManager
	defaultOSReadFileTokenManager      = osReadFileTokenManager
	defaultJSONMarshalTokenManager     = jsonMarshalTokenManager
	defaultJSONUnmarshalTokenManager   = jsonUnmarshalTokenManager
	defaultNewRenewalTicker            = newRenewalTicker
	defaultJSONMarshalAuthentication   = jsonMarshalAuthentication
	defaultCreateAssertionAuthData     = createAssertionAuthData
	defaultCreateAssertionSignature    = createAssertionSignature
	defaultECDSASignAuthentication     = ecdsaSignAuthentication
	defaultGenerateCredentialAuth      = generateCredentialAuthenticator
	defaultCreateAttestationAuth       = createAttestationAuthenticator
	defaultFinishRegistrationAuth      = finishRegistrationAuthenticator
	defaultCreateAuthenticatorDataAuth = createAuthenticatorDataAuthenticator
	defaultCreateAttestationObjectAuth = createAttestationObjectAuthenticator
	defaultJSONMarshalAuthenticator    = jsonMarshalAuthenticator
)

func resetWebAuthnHooks() {
	osMkdirAllTypes = defaultOSMkdirAllTypes
	osReadFileTypes = defaultOSReadFileTypes
	osWriteFileTypes = defaultOSWriteFileTypes
	jsonMarshalIndentTypes = defaultJSONMarshalIndentTypes
	ecdsaGenerateKey = defaultECDSAGenerateKey
	x509MarshalPKCS8PrivateKey = defaultX509MarshalPKCS8PrivateKey
	ioReadFullTypes = defaultIOReadFullTypes
	cborMarshalTypes = defaultCBORMarshalTypes
	x509ParsePKCS8PrivateKey = defaultX509ParsePKCS8PrivateKey
	ecdsaSignASN1Types = defaultECDSASignASN1Types

	osUserHomeDirTokenManager = defaultOSUserHomeDirTokenManager
	osMkdirAllTokenManager = defaultOSMkdirAllTokenManager
	osWriteFileTokenManager = defaultOSWriteFileTokenManager
	osRenameTokenManager = defaultOSRenameTokenManager
	osRemoveTokenManager = defaultOSRemoveTokenManager
	osStatTokenManager = defaultOSStatTokenManager
	osReadFileTokenManager = defaultOSReadFileTokenManager
	jsonMarshalTokenManager = defaultJSONMarshalTokenManager
	jsonUnmarshalTokenManager = defaultJSONUnmarshalTokenManager
	newRenewalTicker = defaultNewRenewalTicker

	jsonMarshalAuthentication = defaultJSONMarshalAuthentication
	createAssertionAuthData = defaultCreateAssertionAuthData
	createAssertionSignature = defaultCreateAssertionSignature
	ecdsaSignAuthentication = defaultECDSASignAuthentication

	generateCredentialAuthenticator = defaultGenerateCredentialAuth
	createAttestationAuthenticator = defaultCreateAttestationAuth
	finishRegistrationAuthenticator = defaultFinishRegistrationAuth
	createAuthenticatorDataAuthenticator = defaultCreateAuthenticatorDataAuth
	createAttestationObjectAuthenticator = defaultCreateAttestationObjectAuth
	jsonMarshalAuthenticator = defaultJSONMarshalAuthenticator
}

func restoreWebAuthnHooks(t *testing.T) {
	t.Helper()
	resetWebAuthnHooks()
	t.Cleanup(resetWebAuthnHooks)
}

func mustCredential(t *testing.T) *Credential {
	t.Helper()
	cred, err := GenerateCredential("hourglass-app.com", base64.RawURLEncoding.EncodeToString([]byte("user")), "User")
	require.NoError(t, err)
	return cred
}

func mustPEMFile(t *testing.T, privateKey any) string {
	t.Helper()
	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "key.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func testBeginRegistrationResponse() *BeginRegistrationResponse {
	return &BeginRegistrationResponse{
		PublicKey: PublicKeyClass{
			Rp:        Rp{Name: "Hourglass", ID: "hourglass-app.com"},
			User:      User{Name: "User", DisplayName: "User", ID: base64.RawURLEncoding.EncodeToString([]byte("user"))},
			Challenge: "challenge",
			AuthenticatorSelection: AuthenticatorSelection{
				AuthenticatorAttachment: "platform",
			},
		},
	}
}

func testBeginAuthenticationResponse() *BeginAuthenticationResponse {
	return &BeginAuthenticationResponse{
		PublicKey: struct {
			Challenge string `json:"challenge"`
			Timeout   int64  `json:"timeout"`
			RpID      string `json:"rpId"`
		}{
			Challenge: "challenge",
			Timeout:   60000,
			RpID:      "hourglass-app.com",
		},
	}
}

func TestEnvironmentCoverage(t *testing.T) {
	t.Run("detects non headless with display", func(t *testing.T) {
		t.Setenv("DISPLAY", ":0")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("CI", "")
		assert.False(t, IsHeadlessEnvironment())
	})

	t.Run("detects ssh headless", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "yes")
		assert.True(t, IsHeadlessEnvironment())
	})

	t.Run("detects ci headless", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("CI", "1")
		assert.True(t, IsHeadlessEnvironment())
	})

	t.Run("defaults to headless without display", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("CI", "")
		assert.True(t, IsHeadlessEnvironment())
	})

	t.Run("HasWebAuthnCredentials branches", func(t *testing.T) {
		assert.False(t, (&TokenManager{}).HasWebAuthnCredentials())

		tm := &TokenManager{storagePath: "\x00/credentials.json"}
		assert.False(t, tm.HasWebAuthnCredentials())

		tm = &TokenManager{storagePath: t.TempDir()}
		assert.False(t, tm.HasWebAuthnCredentials())

		storagePath := filepath.Join(t.TempDir(), "credentials.json")
		storage, err := NewStorage(storagePath)
		require.NoError(t, err)
		stored, err := storage.Load()
		require.NoError(t, err)
		stored.Credentials = append(stored.Credentials, *mustCredential(t))
		require.NoError(t, storage.Save(stored))

		tm = &TokenManager{storagePath: storagePath}
		assert.True(t, tm.HasWebAuthnCredentials())
	})
}

func TestTypesCoverage(t *testing.T) {
	t.Run("NewStorage returns mkdir error", func(t *testing.T) {
		resetWebAuthnHooks()
		osMkdirAllTypes = func(path string, perm os.FileMode) error {
			return errors.New("mkdir failed")
		}

		storage, err := NewStorage(filepath.Join(t.TempDir(), "credentials.json"))
		require.Error(t, err)
		assert.Nil(t, storage)
	})

	t.Run("Storage Load returns read and parse errors", func(t *testing.T) {
		resetWebAuthnHooks()
		storage := &Storage{path: "credentials.json"}
		osReadFileTypes = func(path string) ([]byte, error) {
			return nil, errors.New("read failed")
		}
		_, err := storage.Load()
		require.Error(t, err)

		osReadFileTypes = func(path string) ([]byte, error) {
			return []byte("{"), nil
		}
		_, err = storage.Load()
		require.Error(t, err)
	})

	t.Run("Storage Save returns marshal error", func(t *testing.T) {
		resetWebAuthnHooks()
		storage := &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}
		jsonMarshalIndentTypes = func(v any, prefix, indent string) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}

		err := storage.Save(&StoredCredentials{})
		require.Error(t, err)
	})

	t.Run("GenerateCredential returns wrapped errors", func(t *testing.T) {
		resetWebAuthnHooks()
		ecdsaGenerateKey = func(c elliptic.Curve, r io.Reader) (*ecdsa.PrivateKey, error) {
			return nil, errors.New("key failed")
		}
		_, err := GenerateCredential("hourglass-app.com", "user", "User")
		require.Error(t, err)

		resetWebAuthnHooks()
		validKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		ecdsaGenerateKey = func(c elliptic.Curve, r io.Reader) (*ecdsa.PrivateKey, error) {
			return validKey, nil
		}
		x509MarshalPKCS8PrivateKey = func(key any) ([]byte, error) {
			return nil, errors.New("marshal key failed")
		}
		_, err = GenerateCredential("hourglass-app.com", "user", "User")
		require.Error(t, err)

		resetWebAuthnHooks()
		validKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		ecdsaGenerateKey = func(c elliptic.Curve, r io.Reader) (*ecdsa.PrivateKey, error) {
			return validKey, nil
		}
		ioReadFullTypes = func(r io.Reader, buf []byte) (int, error) {
			return 0, errors.New("readfull failed")
		}
		_, err = GenerateCredential("hourglass-app.com", "user", "User")
		require.Error(t, err)

		resetWebAuthnHooks()
		validKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		ecdsaGenerateKey = func(c elliptic.Curve, r io.Reader) (*ecdsa.PrivateKey, error) {
			return validKey, nil
		}
		ioReadFullTypes = func(r io.Reader, buf []byte) (int, error) {
			for i := range buf {
				buf[i] = byte(i + 1)
			}
			return len(buf), nil
		}
		cborMarshalTypes = func(v any) ([]byte, error) {
			return nil, errors.New("cbor failed")
		}
		_, err = GenerateCredential("hourglass-app.com", "user", "User")
		require.Error(t, err)
	})

	t.Run("LoadCredentialFromPEM wraps marshal and COSE errors", func(t *testing.T) {
		resetWebAuthnHooks()
		pemPath := mustPEMFile(t, mustCredential(t).GetPrivateKeyOrPanic(t))
		x509MarshalPKCS8PrivateKey = func(key any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}
		_, err := LoadCredentialFromPEM(pemPath, "cred-id", "hourglass-app.com", "dXNlcg", "User")
		require.Error(t, err)

		resetWebAuthnHooks()
		pemPath = mustPEMFile(t, mustCredential(t).GetPrivateKeyOrPanic(t))
		cborMarshalTypes = func(v any) ([]byte, error) {
			return nil, errors.New("cbor failed")
		}
		_, err = LoadCredentialFromPEM(pemPath, "cred-id", "hourglass-app.com", "dXNlcg", "User")
		require.Error(t, err)
	})

	t.Run("GetPrivateKey and Sign return wrapped errors", func(t *testing.T) {
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
		require.NoError(t, err)

		cred := &Credential{PrivateKey: pkcs8}
		_, err = cred.GetPrivateKey()
		require.Error(t, err)

		resetWebAuthnHooks()
		validCred := mustCredential(t)
		ecdsaSignASN1Types = func(rand io.Reader, priv *ecdsa.PrivateKey, hash []byte) ([]byte, error) {
			return nil, errors.New("sign failed")
		}
		_, err = validCred.Sign(nil, []byte("12345678901234567890123456789012"))
		require.Error(t, err)
	})
}

func TestAuthenticationCoverage(t *testing.T) {
	t.Run("stored credential helpers return errors", func(t *testing.T) {
		auth := &Authenticator{storage: &Storage{path: t.TempDir()}}
		_, err := auth.getStoredCredential()
		require.Error(t, err)

		err = auth.updateStoredCredential(&Credential{ID: "missing"})
		require.Error(t, err)

		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		storage, err := NewStorage(storagePath)
		require.NoError(t, err)
		stored, err := storage.Load()
		require.NoError(t, err)
		stored.Credentials = append(stored.Credentials, *mustCredential(t))
		require.NoError(t, storage.Save(stored))

		auth = &Authenticator{storage: storage}
		err = auth.updateStoredCredential(&Credential{ID: "other"})
		require.Error(t, err)
	})

	t.Run("beginAuthentication forwards existing cookie", func(t *testing.T) {
		auth := &Authenticator{
			baseURL: "https://example.com",
			hgLogin: "existing-hg",
			httpClient: &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
				cookie, err := req.Cookie("hglogin")
				require.NoError(t, err)
				assert.Equal(t, "existing-hg", cookie.Value)

				header := make(http.Header)
				header.Add("Set-Cookie", "hglogin=updated-hg")
				header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=updated-xsrf")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"challenge":"c","timeout":1,"rpId":"hourglass-app.com"}}`)),
					Header:     header,
				}, nil
			}},
		}

		resp, err := auth.beginAuthentication()
		require.NoError(t, err)
		assert.Equal(t, "c", resp.PublicKey.Challenge)
		assert.Equal(t, "updated-hg", auth.hgLogin)
		assert.Equal(t, "updated-xsrf", auth.xsrfToken)
	})

	t.Run("createAssertion wraps marshal and helper errors", func(t *testing.T) {
		resetWebAuthnHooks()
		auth := &Authenticator{}
		cred := mustCredential(t)
		jsonMarshalAuthentication = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}
		_, err := auth.createAssertion(cred, testBeginAuthenticationResponse())
		require.Error(t, err)

		resetWebAuthnHooks()
		createAssertionAuthData = func(a *Authenticator, cred *Credential, clientDataHash []byte) ([]byte, error) {
			return nil, errors.New("authdata failed")
		}
		_, err = auth.createAssertion(cred, testBeginAuthenticationResponse())
		require.Error(t, err)

		resetWebAuthnHooks()
		createAssertionSignature = func(a *Authenticator, privateKey *ecdsa.PrivateKey, authData, clientDataHash []byte) ([]byte, error) {
			return nil, errors.New("signature failed")
		}
		_, err = auth.createAssertion(cred, testBeginAuthenticationResponse())
		require.Error(t, err)
	})

	t.Run("createSignature returns sign error", func(t *testing.T) {
		resetWebAuthnHooks()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		ecdsaSignAuthentication = func(rand io.Reader, priv *ecdsa.PrivateKey, hash []byte) (*big.Int, *big.Int, error) {
			return nil, nil, errors.New("sign failed")
		}

		_, err = (&Authenticator{}).createSignature(key, []byte("auth-data"), []byte("client-data"))
		require.Error(t, err)
	})

	t.Run("Authenticate wraps finish error", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		auth, err := NewAuthenticator(storagePath, "https://example.com")
		require.NoError(t, err)

		stored, err := auth.storage.Load()
		require.NoError(t, err)
		stored.Credentials = append(stored.Credentials, *mustCredential(t))
		require.NoError(t, auth.storage.Save(stored))

		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/v0.2/auth/webauthn/login/begin":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"challenge":"c","timeout":1,"rpId":"hourglass-app.com"}}`)),
					Header:     make(http.Header),
				}, nil
			case "/api/v0.2/auth/webauthn/login/finish":
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(strings.NewReader("denied")),
					Header:     make(http.Header),
				}, nil
			default:
				return nil, errors.New("unexpected request")
			}
		}}

		_, err = auth.Authenticate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "finish authentication failed")
	})

	t.Run("Authenticate wraps update error", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		auth, err := NewAuthenticator(storagePath, "https://example.com")
		require.NoError(t, err)

		stored, err := auth.storage.Load()
		require.NoError(t, err)
		stored.Credentials = append(stored.Credentials, *mustCredential(t))
		require.NoError(t, auth.storage.Save(stored))

		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/v0.2/auth/webauthn/login/begin":
				header := make(http.Header)
				header.Add("Set-Cookie", "hglogin=begin-hg")
				header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=begin-xsrf")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"publicKey":{"challenge":"c","timeout":1,"rpId":"hourglass-app.com"}}`)),
					Header:     header,
				}, nil
			case "/api/v0.2/auth/webauthn/login/finish":
				auth.storage.path = tempDir
				header := make(http.Header)
				header.Add("Set-Cookie", "hglogin=done-hg")
				header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=done-xsrf")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     header,
				}, nil
			default:
				return nil, errors.New("unexpected request")
			}
		}}

		_, err = auth.Authenticate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update credential failed")
	})

	t.Run("finishAuthentication request creation error", func(t *testing.T) {
		auth := &Authenticator{baseURL: "://bad-url", httpClient: defaultHTTPClient}
		_, err := auth.finishAuthentication(&AssertionResponse{})
		require.Error(t, err)
	})
}

func TestAuthenticatorCoverage(t *testing.T) {
	t.Run("NewAuthenticator returns storage error", func(t *testing.T) {
		resetWebAuthnHooks()
		osMkdirAllTypes = func(path string, perm os.FileMode) error {
			return errors.New("mkdir failed")
		}

		auth, err := NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.Error(t, err)
		assert.Nil(t, auth)
	})

	t.Run("Register wraps branch errors", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "credentials.json")
		auth, err := NewAuthenticator(storagePath, "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("begin failed")
		}}
		_, err = auth.Register("User")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth, err = NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			body, marshalErr := json.Marshal(BeginRegistrationResponse{
				PublicKey: PublicKeyClass{
					Rp:        Rp{Name: "Hourglass"},
					User:      User{Name: "User", DisplayName: "User"},
					Challenge: "challenge",
				},
			})
			require.NoError(t, marshalErr)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		}}
		finishRegistrationAuthenticator = func(a *Authenticator, attestation *AttestationResponse) error { return nil }
		cred, err := auth.Register("User")
		require.NoError(t, err)
		assert.NotEmpty(t, cred.UserID)
		assert.Equal(t, "hourglass-app.com", cred.RPID)

		restoreWebAuthnHooks(t)
		auth, err = NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			body, marshalErr := json.Marshal(*testBeginRegistrationResponse())
			require.NoError(t, marshalErr)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		}}
		generateCredentialAuthenticator = func(rpID, userID, userName string) (*Credential, error) {
			return nil, errors.New("generate failed")
		}
		_, err = auth.Register("User")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth, err = NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			body, marshalErr := json.Marshal(*testBeginRegistrationResponse())
			require.NoError(t, marshalErr)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		}}
		createAttestationAuthenticator = func(a *Authenticator, cred *Credential, beginResp *BeginRegistrationResponse) (*AttestationResponse, error) {
			return nil, errors.New("attestation failed")
		}
		_, err = auth.Register("User")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth, err = NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			body, marshalErr := json.Marshal(*testBeginRegistrationResponse())
			require.NoError(t, marshalErr)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		}}
		finishRegistrationAuthenticator = func(a *Authenticator, attestation *AttestationResponse) error {
			return errors.New("finish failed")
		}
		_, err = auth.Register("User")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth, err = NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			body, marshalErr := json.Marshal(*testBeginRegistrationResponse())
			require.NoError(t, marshalErr)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		}}
		finishRegistrationAuthenticator = func(a *Authenticator, attestation *AttestationResponse) error {
			a.storage.path = tempDir
			return nil
		}
		_, err = auth.Register("User")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth, err = NewAuthenticator(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			body, marshalErr := json.Marshal(*testBeginRegistrationResponse())
			require.NoError(t, marshalErr)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
		}}
		osWriteFileTypes = func(path string, data []byte, perm os.FileMode) error {
			return errors.New("write failed")
		}
		_, err = auth.Register("User")
		require.Error(t, err)
	})

	t.Run("beginRegistration and createAttestation branches", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		auth := &Authenticator{baseURL: "https://example.com"}
		auth.SetCookies("xsrf", "hglogin")
		setMockExecCommand(t, `{"publicKey":{"rp":{"name":"Hourglass","id":"hourglass-app.com"},"user":{"name":"User","displayName":"User","id":"dXNlcg"},"challenge":"challenge"}}`+"\n200", 0)
		beginResp, err := auth.beginRegistration("User")
		require.NoError(t, err)
		assert.Equal(t, "challenge", beginResp.PublicKey.Challenge)

		restoreWebAuthnHooks(t)
		auth = &Authenticator{baseURL: "https://example.com"}
		auth.SetCookies("xsrf", "hglogin")
		setMockExecCommand(t, "{\n200", 0)
		_, err = auth.beginRegistration("User")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth = &Authenticator{}
		cred := mustCredential(t)
		jsonMarshalAuthenticator = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}
		_, err = auth.createAttestation(cred, testBeginRegistrationResponse())
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		createAuthenticatorDataAuthenticator = func(a *Authenticator, cred *Credential, clientDataHash []byte) ([]byte, error) {
			return nil, errors.New("authdata failed")
		}
		_, err = auth.createAttestation(cred, testBeginRegistrationResponse())
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		createAttestationObjectAuthenticator = func(a *Authenticator, authData []byte) ([]byte, error) {
			return nil, errors.New("attestation object failed")
		}
		_, err = auth.createAttestation(cred, testBeginRegistrationResponse())
		require.Error(t, err)

		cred.ID = "%%%invalid%%%"
		_, err = auth.createAuthenticatorData(cred, []byte("hash"))
		require.Error(t, err)
	})

	t.Run("finishRegistration branches", func(t *testing.T) {
		auth := &Authenticator{}
		err := auth.finishRegistration(&AttestationResponse{
			Type:                   "public-key",
			ClientExtensionResults: map[string]any{"bad": make(chan int)},
		})
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		auth = &Authenticator{baseURL: "https://example.com"}
		auth.SetCookies("xsrf", "hglogin")
		setMockExecCommand(t, "\n201", 0)
		err = auth.finishRegistration(&AttestationResponse{Type: "public-key"})
		require.NoError(t, err)
	})
}

func TestTokenManagerCoverage(t *testing.T) {
	t.Run("NewTokenManager covers home, error and headless branches", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
		t.Setenv("WEBAUTHN_TOKENS_PATH", "")
		t.Setenv("DISPLAY", ":0")
		osUserHomeDirTokenManager = func() (string, error) {
			return t.TempDir(), nil
		}

		tm, err := NewTokenManager("", "https://example.com", WithBrowserAuth(nil))
		require.NoError(t, err)
		assert.Contains(t, tm.storagePath, ".hourglass-rpa")

		restoreWebAuthnHooks(t)
		osMkdirAllTypes = func(path string, perm os.FileMode) error {
			return errors.New("mkdir failed")
		}
		_, err = NewTokenManager(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.Error(t, err)

		restoreWebAuthnHooks(t)
		t.Setenv("DISPLAY", "")
		t.Setenv("CI", "1")
		tm, err = NewTokenManager(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com")
		require.NoError(t, err)
		assert.Nil(t, tm.browserAuth)
	})

	t.Run("Start and EnsureValidTokens cover remaining branches", func(t *testing.T) {
		tm, err := NewTokenManager(filepath.Join(t.TempDir(), "credentials.json"), "https://example.com", WithBrowserAuth(nil))
		require.NoError(t, err)
		err = tm.Start(context.Background())
		require.Error(t, err)

		tm = &TokenManager{
			renewalThreshold: time.Hour,
			stopChan:         make(chan struct{}),
		}
		tm.setTokens(&AuthTokens{HGLogin: "stale", XSRFToken: "stale", ExpiresAt: time.Now().Add(10 * time.Minute)})
		tm.renewMu.Lock()
		done := make(chan struct{})
		go func() {
			defer close(done)
			tokens, ensureErr := tm.EnsureValidTokens()
			require.NoError(t, ensureErr)
			assert.Equal(t, "fresh", tokens.HGLogin)
		}()
		time.Sleep(20 * time.Millisecond)
		tm.setTokens(&AuthTokens{HGLogin: "fresh", XSRFToken: "fresh", ExpiresAt: time.Now().Add(2 * time.Hour)})
		tm.renewMu.Unlock()
		<-done

		tm = &TokenManager{
			authenticator:    &Authenticator{storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}},
			renewalThreshold: time.Hour,
			stopChan:         make(chan struct{}),
			currentTokens:    &AuthTokens{HGLogin: "partial"},
		}
		_, err = tm.EnsureValidTokens()
		require.Error(t, err)

		tm.currentTokens = &AuthTokens{HGLogin: "valid", XSRFToken: "valid", ExpiresAt: time.Now().Add(10 * time.Minute)}
		_, err = tm.EnsureValidTokens()
		require.Error(t, err)
	})

	t.Run("SaveTokens and LoadTokens cover error branches", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		tm := &TokenManager{tokensPath: filepath.Join(t.TempDir(), "tokens.json")}
		tokens := &AuthTokens{HGLogin: "h", XSRFToken: "x", ExpiresAt: time.Now().Add(time.Hour)}

		jsonMarshalTokenManager = func(v any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}
		require.Error(t, tm.SaveTokens(tokens))

		restoreWebAuthnHooks(t)
		osMkdirAllTokenManager = func(path string, perm os.FileMode) error {
			return errors.New("mkdir failed")
		}
		require.Error(t, tm.SaveTokens(tokens))

		restoreWebAuthnHooks(t)
		osWriteFileTokenManager = func(path string, data []byte, perm os.FileMode) error {
			return errors.New("write failed")
		}
		require.Error(t, tm.SaveTokens(tokens))

		restoreWebAuthnHooks(t)
		removed := false
		osRenameTokenManager = func(oldpath, newpath string) error {
			return errors.New("rename failed")
		}
		osRemoveTokenManager = func(path string) error {
			removed = true
			return nil
		}
		require.Error(t, tm.SaveTokens(tokens))
		assert.True(t, removed)

		restoreWebAuthnHooks(t)
		osStatTokenManager = func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}
		require.NoError(t, tm.SaveTokens(tokens))

		tm.tokensPath = ""
		_, err := tm.LoadTokens()
		require.Error(t, err)

		tm.tokensPath = t.TempDir()
		_, err = tm.LoadTokens()
		require.Error(t, err)

		tm.tokensPath = filepath.Join(t.TempDir(), "tokens.json")
		require.NoError(t, os.WriteFile(tm.tokensPath, []byte("{"), 0o600))
		_, err = tm.LoadTokens()
		require.Error(t, err)
	})

	t.Run("token renewal helpers and browser fallback branches", func(t *testing.T) {
		assert.True(t, (&TokenManager{renewalThreshold: time.Hour}).tokensNeedRenewal(&AuthTokens{HGLogin: "partial"}))
		assert.Equal(t, "", normalizeWebAuthnBaseURL(""))
		assert.Equal(t, "://bad-url", normalizeWebAuthnBaseURL("://bad-url"))

		restoreBrowserAuthHooks(t)
		restoreWebAuthnHooks(t)
		t.Setenv("CHROME_BIN", "/tmp/chrome")
		chromedpRun = func(ctx context.Context, actions ...chromedp.Action) error {
			return nil
		}
		evaluateAuthPageState = func(ctx context.Context, state *authPageState) error {
			*state = authPageState{HasAuthButton: true}
			return nil
		}
		triggerWebAuthnPrompt = func(ctx context.Context, clicked *bool) error {
			*clicked = true
			return nil
		}
		getCookies = func(ctx context.Context) ([]*network.Cookie, error) {
			return browserAuthCookies(), nil
		}

		tm := &TokenManager{
			browserAuth:      NewBrowserAuth("https://example.com"),
			authenticator:    &Authenticator{storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}},
			renewalThreshold: time.Hour,
			stopChan:         make(chan struct{}),
			tokensPath:       "",
			currentTokens:    nil,
			storagePath:      filepath.Join(t.TempDir(), "credentials.json"),
		}

		tokens, err := tm.EnsureValidTokens()
		require.NoError(t, err)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)

		tokens, err = tm.authenticateWithFallback()
		require.NoError(t, err)
		assert.Equal(t, "test-xsrf", tokens.XSRFToken)
	})

	t.Run("renewalLoop logs errors on tick", func(t *testing.T) {
		restoreWebAuthnHooks(t)
		fakeTicker := &fakeRenewalTicker{ch: make(chan time.Time, 1)}
		newRenewalTicker = func(d time.Duration) renewalTicker {
			return fakeTicker
		}

		tm := &TokenManager{
			authenticator:    &Authenticator{storage: &Storage{path: filepath.Join(t.TempDir(), "credentials.json")}},
			renewalThreshold: time.Hour,
			stopChan:         make(chan struct{}),
			currentTokens:    &AuthTokens{HGLogin: "partial"},
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			tm.renewalLoop(context.Background())
		}()

		fakeTicker.ch <- time.Now()
		time.Sleep(20 * time.Millisecond)
		close(tm.stopChan)
		<-done
	})
}

func (c *Credential) GetPrivateKeyOrPanic(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := c.GetPrivateKey()
	require.NoError(t, err)
	return key
}
