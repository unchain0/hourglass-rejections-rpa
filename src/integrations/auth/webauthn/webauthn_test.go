package webauthn

import (
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeECDSASignature_UsesASN1DERExpectedByWebAuthn(t *testing.T) {
	signature, err := encodeECDSASignature(big.NewInt(1), big.NewInt(2))
	require.NoError(t, err)

	var decoded struct {
		R *big.Int
		S *big.Int
	}
	rest, err := asn1.Unmarshal(signature, &decoded)
	require.NoError(t, err)
	assert.Empty(t, rest)
	assert.Equal(t, big.NewInt(1), decoded.R)
	assert.Equal(t, big.NewInt(2), decoded.S)
}

func TestCreateAssertionAuthenticatorData_ReportsOnlyUserPresence(t *testing.T) {
	cred, err := GenerateCredential("hourglass-app.com", "dGVzdC11c2Vy", "Test User")
	require.NoError(t, err)

	authData, err := (&Authenticator{}).createAssertionAuthenticatorData(cred, nil)
	require.NoError(t, err)
	require.Len(t, authData, 37)
	assert.Equal(t, byte(0x01), authData[32])
}

func TestCredential_GetUserIDBytes_DecodesWebAuthnBase64URL(t *testing.T) {
	want := []byte{0xfb, 0xff, 0xef, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}
	cred := &Credential{UserID: base64.RawURLEncoding.EncodeToString(want)}

	got, err := cred.GetUserIDBytes()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGenerateCredential(t *testing.T) {
	rpID := "hourglass-app.com"
	userID := "amTw3aU4QEKsn1pyxS7jiw"
	userName := "Test User"

	cred, err := GenerateCredential(rpID, userID, userName)
	require.NoError(t, err)
	require.NotNil(t, cred)

	assert.NotEmpty(t, cred.ID)
	assert.NotEmpty(t, cred.PrivateKey)
	assert.NotEmpty(t, cred.PublicKey)
	assert.Equal(t, rpID, cred.RPID)
	assert.Equal(t, userID, cred.UserID)
	assert.Equal(t, userName, cred.UserName)
	assert.Equal(t, uint32(0), cred.SignCount)
	assert.False(t, cred.CreatedAt.IsZero())

	privateKey, err := cred.GetPrivateKey()
	require.NoError(t, err)
	require.NotNil(t, privateKey)

	credIDBytes, err := cred.GetCredentialIDBytes()
	require.NoError(t, err)
	assert.Len(t, credIDBytes, 16)
}

func TestStorage(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test-credentials.json")

	storage, err := NewStorage(storagePath)
	require.NoError(t, err)

	creds, err := storage.Load()
	require.NoError(t, err)
	assert.Empty(t, creds.Credentials)
	assert.Equal(t, 1, creds.Version)

	cred, err := GenerateCredential("hourglass-app.com", "test-user-id", "Test User")
	require.NoError(t, err)

	creds.Credentials = append(creds.Credentials, *cred)
	err = storage.Save(creds)
	require.NoError(t, err)

	info, err := os.Stat(storagePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loadedCreds, err := storage.Load()
	require.NoError(t, err)
	require.Len(t, loadedCreds.Credentials, 1)
	assert.Equal(t, cred.ID, loadedCreds.Credentials[0].ID)
	assert.Equal(t, cred.UserID, loadedCreds.Credentials[0].UserID)
}

func TestAuthTokens(t *testing.T) {
	tokens := &AuthTokens{
		HGLogin:   "test-hglogin-cookie",
		XSRFToken: "test-xsrf-token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	assert.False(t, tokens.IsExpired())
	assert.False(t, tokens.IsNearExpiry(30*time.Minute))
	assert.True(t, tokens.IsNearExpiry(2*time.Hour))
	assert.True(t, tokens.IsUsable())

	expiredTokens := &AuthTokens{
		HGLogin:   "test",
		XSRFToken: "test",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	assert.True(t, expiredTokens.IsExpired())
	assert.True(t, expiredTokens.IsUsable())

	incompleteTokens := &AuthTokens{HGLogin: "test"}
	assert.False(t, incompleteTokens.IsUsable())
}

func TestAuthenticator_Register_UsesHourglassAPIPath(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")
	auth, err := NewAuthenticator(storagePath, "https://example.com")
	require.NoError(t, err)
	auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v0.2/auth/webauthn/register/begin":
			response := BeginRegistrationResponse{
				PublicKey: PublicKeyClass{
					Rp: Rp{
						Name: "Hourglass",
						ID:   "hourglass-app.com",
					},
					User: User{
						Name:        "Test User",
						DisplayName: "Test User",
						ID:          "test-user-id",
					},
					Challenge: "8fh9areIeMAp8-11MYIHWOYPy82Dku5krP-AJpyFJjM",
					PubKeyCredParams: []PubKeyCredParam{
						{Type: "public-key", Alg: -7},
					},
					Timeout:            60000,
					ExcludeCredentials: []ExcludeCredential{},
					AuthenticatorSelection: AuthenticatorSelection{
						AuthenticatorAttachment: "platform",
					},
				},
			}
			body, err := json.Marshal(response)
			require.NoError(t, err)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Header:     make(http.Header),
			}, nil
		case "/api/v0.2/auth/webauthn/register/finish":
			var reqBody AttestationResponse
			err := json.NewDecoder(req.Body).Decode(&reqBody)
			require.NoError(t, err)
			assert.Equal(t, "public-key", reqBody.Type)
			assert.NotEmpty(t, reqBody.ID)
			assert.NotEmpty(t, reqBody.Response.ClientDataJSON)
			assert.NotEmpty(t, reqBody.Response.AttestationObject)
			assert.Equal(t, "platform", reqBody.AuthenticatorAttachment)
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
	require.NotNil(t, cred)

	storedCreds, err := auth.storage.Load()
	require.NoError(t, err)
	require.Len(t, storedCreds.Credentials, 1)
	assert.Equal(t, cred.ID, storedCreds.Credentials[0].ID)
}

func TestCreateAttestationObject(t *testing.T) {
	authData := []byte{1, 2, 3}
	auth := &Authenticator{}

	encoded, err := auth.createAttestationObject(authData)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, cbor.Unmarshal(encoded, &decoded))
	assert.Equal(t, "none", decoded["fmt"])
	assert.Equal(t, authData, decoded["authData"])
	assert.Equal(t, map[any]any{}, decoded["attStmt"])
}

func TestAuthenticator_Authenticate(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	cred, err := GenerateCredential("hourglass-app.com", "test-user-id", "Test User")
	require.NoError(t, err)

	storage, err := NewStorage(storagePath)
	require.NoError(t, err)

	storedCreds, err := storage.Load()
	require.NoError(t, err)
	storedCreds.Credentials = append(storedCreds.Credentials, *cred)
	err = storage.Save(storedCreds)
	require.NoError(t, err)

	auth, err := NewAuthenticator(storagePath, "https://example.com")
	require.NoError(t, err)
	auth.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v0.2/auth/webauthn/login/begin":
			assert.Equal(t, http.MethodGet, req.Method)
			response := BeginAuthenticationResponse{
				PublicKey: struct {
					Challenge string `json:"challenge"`
					Timeout   int64  `json:"timeout"`
					RpID      string `json:"rpId"`
				}{
					Challenge: "test-challenge-123",
					RpID:      "hourglass-app.com",
					Timeout:   60000,
				},
			}
			body, err := json.Marshal(response)
			require.NoError(t, err)
			header := make(http.Header)
			header.Add("Set-Cookie", "hglogin=begin-hg")
			header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=begin-xsrf")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Header:     header,
			}, nil
		case "/api/v0.2/auth/webauthn/login/finish":
			var reqBody AssertionResponse
			err := json.NewDecoder(req.Body).Decode(&reqBody)
			require.NoError(t, err)
			assert.Equal(t, "public-key", reqBody.Type)
			assert.NotEmpty(t, reqBody.ID)
			assert.NotEmpty(t, reqBody.Response.AuthenticatorData)
			assert.NotEmpty(t, reqBody.Response.ClientDataJSON)
			assert.NotEmpty(t, reqBody.Response.Signature)
			assert.NotEmpty(t, reqBody.Response.UserHandle)
			cookie, err := req.Cookie("hglogin")
			require.NoError(t, err)
			assert.Equal(t, "begin-hg", cookie.Value)
			assert.Equal(t, "begin-xsrf", req.Header.Get("X-Hourglass-XSRF-Token"))

			header := make(http.Header)
			header.Add("Set-Cookie", "hglogin=test-hglogin-value")
			header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=test-xsrf-token")
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

	tokens, err := auth.Authenticate()
	require.NoError(t, err)
	require.NotNil(t, tokens)

	assert.Equal(t, "test-hglogin-value", tokens.HGLogin)
	assert.Equal(t, "test-xsrf-token", tokens.XSRFToken)
	assert.False(t, tokens.IsExpired())

	updatedCreds, err := auth.storage.Load()
	require.NoError(t, err)
	assert.Equal(t, uint32(1), updatedCreds.Credentials[0].SignCount)
}

func TestTokenManager(t *testing.T) {
	authCallCount := 0
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	userID := base64.RawURLEncoding.EncodeToString([]byte("test-user"))
	cred, err := GenerateCredential("hourglass-app.com", userID, "Test")
	require.NoError(t, err)
	storage, err := NewStorage(storagePath)
	require.NoError(t, err)
	storedCreds, err := storage.Load()
	require.NoError(t, err)
	storedCreds.Credentials = append(storedCreds.Credentials, *cred)
	require.NoError(t, storage.Save(storedCreds))

	tokenRenewed := false
	tm, err := NewTokenManager(storagePath, "https://example.com",
		WithBrowserAuth(nil),
		WithRenewalThreshold(2*time.Hour),
		WithOnTokenRenewed(func(tokens *AuthTokens) {
			tokenRenewed = true
		}),
	)
	require.NoError(t, err)
	tm.authenticator.httpClient = &mockHTTPClient{do: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v0.2/auth/webauthn/login/begin":
			response := BeginAuthenticationResponse{
				PublicKey: struct {
					Challenge string `json:"challenge"`
					Timeout   int64  `json:"timeout"`
					RpID      string `json:"rpId"`
				}{
					Challenge: "test-challenge",
					RpID:      "hourglass-app.com",
					Timeout:   60000,
				},
			}
			body, err := json.Marshal(response)
			require.NoError(t, err)
			header := make(http.Header)
			header.Add("Set-Cookie", "hglogin=pre-auth-hg")
			header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=pre-auth-xsrf")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Header:     header,
			}, nil
		case "/api/v0.2/auth/webauthn/login/finish":
			authCallCount++
			header := make(http.Header)
			header.Add("Set-Cookie", "hglogin=token-value")
			header.Add("Set-Cookie", "X-Hourglass-XSRF-Token=xsrf-value")
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

	tokens, err := tm.EnsureValidTokens()
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.True(t, tokenRenewed)
	assert.Equal(t, 1, authCallCount)

	tokens2, err := tm.EnsureValidTokens()
	require.NoError(t, err)
	assert.Equal(t, tokens.HGLogin, tokens2.HGLogin)
	assert.Equal(t, 1, authCallCount)

	assert.True(t, tm.IsAuthenticated())

	retrieved := tm.GetTokens()
	assert.Equal(t, tokens.HGLogin, retrieved.HGLogin)
}

func TestCredential_IncrementSignCount(t *testing.T) {
	cred, err := GenerateCredential("test.com", "user", "Test")
	require.NoError(t, err)

	assert.Equal(t, uint32(0), cred.SignCount)
	assert.True(t, cred.LastUsedAt.IsZero())

	cred.incrementSignCount()

	assert.Equal(t, uint32(1), cred.SignCount)
	assert.False(t, cred.LastUsedAt.IsZero())

	cred.incrementSignCount()
	assert.Equal(t, uint32(2), cred.SignCount)
}

func TestBase64Decoding(t *testing.T) {
	data := []byte("test data")
	encoded := base64.StdEncoding.EncodeToString(data)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, data, decoded)

	urlEncoded := base64.RawURLEncoding.EncodeToString(data)
	urlDecoded, err := base64.RawURLEncoding.DecodeString(urlEncoded)
	require.NoError(t, err)
	assert.Equal(t, data, urlDecoded)
}
