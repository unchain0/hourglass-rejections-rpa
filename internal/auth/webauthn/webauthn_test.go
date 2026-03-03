package webauthn

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Verify we can get the private key back
	privateKey, err := cred.GetPrivateKey()
	require.NoError(t, err)
	require.NotNil(t, privateKey)

	// Verify credential ID is valid base64
	credIDBytes, err := cred.GetCredentialIDBytes()
	require.NoError(t, err)
	assert.Len(t, credIDBytes, 16)
}

func TestStorage(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test-credentials.json")

	storage, err := NewStorage(storagePath)
	require.NoError(t, err)

	// Test loading empty storage
	creds, err := storage.Load()
	require.NoError(t, err)
	assert.Empty(t, creds.Credentials)
	assert.Equal(t, 1, creds.Version)

	// Test saving credentials
	cred, err := GenerateCredential("hourglass-app.com", "test-user-id", "Test User")
	require.NoError(t, err)

	creds.Credentials = append(creds.Credentials, *cred)
	err = storage.Save(creds)
	require.NoError(t, err)

	// Verify file has correct permissions
	info, err := os.Stat(storagePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Test loading saved credentials
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

	expiredTokens := &AuthTokens{
		HGLogin:   "test",
		XSRFToken: "test",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	assert.True(t, expiredTokens.IsExpired())
}

func TestAuthenticator_Register(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/webauthn/register/begin":
			// Return mock begin response
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
			json.NewEncoder(w).Encode(response)

		case "/auth/webauthn/register/finish":
			// Verify request structure
			var req AttestationResponse
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)

			assert.Equal(t, "public-key", req.Type)
			assert.NotEmpty(t, req.ID)
			assert.NotEmpty(t, req.Response.ClientDataJSON)
			assert.NotEmpty(t, req.Response.AttestationObject)
			assert.Equal(t, "platform", req.AuthenticatorAttachment)

			w.WriteHeader(http.StatusCreated)

		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create temp storage
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	// Create authenticator
	auth, err := NewAuthenticator(storagePath, server.URL)
	require.NoError(t, err)

	// Test registration
	cred, err := auth.Register("Test User")
	require.NoError(t, err)
	require.NotNil(t, cred)

	// Verify credential was stored
	storedCreds, err := auth.storage.Load()
	require.NoError(t, err)
	require.Len(t, storedCreds.Credentials, 1)
	assert.Equal(t, cred.ID, storedCreds.Credentials[0].ID)
}

func TestAuthenticator_Authenticate(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/webauthn/login/begin":
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
			json.NewEncoder(w).Encode(response)

		case "/auth/webauthn/login/finish":
			var req AssertionResponse
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)

			assert.Equal(t, "public-key", req.Type)
			assert.NotEmpty(t, req.ID)
			assert.NotEmpty(t, req.Response.AuthenticatorData)
			assert.NotEmpty(t, req.Response.ClientDataJSON)
			assert.NotEmpty(t, req.Response.Signature)
			assert.NotEmpty(t, req.Response.UserHandle)

			// Set cookies in response
			http.SetCookie(w, &http.Cookie{
				Name:  "hglogin",
				Value: "test-hglogin-value",
			})
			http.SetCookie(w, &http.Cookie{
				Name:  "X-Hourglass-XSRF-Token",
				Value: "test-xsrf-token",
			})
			w.WriteHeader(http.StatusOK)

		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create temp storage with a pre-registered credential
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	cred, err := GenerateCredential("hourglass-app.com", "test-user-id", "Test User")
	require.NoError(t, err)

	storage, err := NewStorage(storagePath)
	require.NoError(t, err)

	storedCreds, _ := storage.Load()
	storedCreds.Credentials = append(storedCreds.Credentials, *cred)
	err = storage.Save(storedCreds)
	require.NoError(t, err)

	// Create authenticator
	auth, err := NewAuthenticator(storagePath, server.URL)
	require.NoError(t, err)

	// Test authentication
	tokens, err := auth.Authenticate()
	require.NoError(t, err)
	require.NotNil(t, tokens)

	assert.Equal(t, "test-hglogin-value", tokens.HGLogin)
	assert.Equal(t, "test-xsrf-token", tokens.XSRFToken)
	assert.False(t, tokens.IsExpired())

	// Verify sign count was incremented
	updatedCreds, _ := auth.storage.Load()
	assert.Equal(t, uint32(1), updatedCreds.Credentials[0].SignCount)
}

func TestTokenManager(t *testing.T) {
	// Create mock server
	authCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/webauthn/login/begin":
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
			json.NewEncoder(w).Encode(response)

		case "/auth/webauthn/login/finish":
			authCallCount++
			http.SetCookie(w, &http.Cookie{Name: "hglogin", Value: "token-" + string(rune(authCallCount))})
			http.SetCookie(w, &http.Cookie{Name: "X-Hourglass-XSRF-Token", Value: "xsrf-" + string(rune(authCallCount))})
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Create temp storage with credential
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "credentials.json")

	cred, _ := GenerateCredential("hourglass-app.com", "test-user", "Test")
	storage, _ := NewStorage(storagePath)
	storedCreds, _ := storage.Load()
	storedCreds.Credentials = append(storedCreds.Credentials, *cred)
	storage.Save(storedCreds)

	// Create token manager
	tokenRenewed := false
	tm, err := NewTokenManager(storagePath, server.URL,
		WithRenewalThreshold(2*time.Hour), // Long threshold for testing
		WithOnTokenRenewed(func(tokens *AuthTokens) {
			tokenRenewed = true
		}),
	)
	require.NoError(t, err)

	// Test initial authentication
	tokens, err := tm.EnsureValidTokens()
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.True(t, tokenRenewed)
	assert.Equal(t, 1, authCallCount)

	// Test that subsequent calls don't re-authenticate (tokens still valid)
	tokens2, err := tm.EnsureValidTokens()
	require.NoError(t, err)
	assert.Equal(t, tokens.HGLogin, tokens2.HGLogin)
	assert.Equal(t, 1, authCallCount) // Should not have called auth again

	// Test IsAuthenticated
	assert.True(t, tm.IsAuthenticated())

	// Test GetTokens returns copy
	retrieved := tm.GetTokens()
	assert.Equal(t, tokens.HGLogin, retrieved.HGLogin)
}

func TestCredential_IncrementSignCount(t *testing.T) {
	cred, _ := GenerateCredential("test.com", "user", "Test")

	assert.Equal(t, uint32(0), cred.SignCount)
	assert.True(t, cred.LastUsedAt.IsZero())

	cred.incrementSignCount()

	assert.Equal(t, uint32(1), cred.SignCount)
	assert.False(t, cred.LastUsedAt.IsZero())

	cred.incrementSignCount()
	assert.Equal(t, uint32(2), cred.SignCount)
}

func TestBase64Decoding(t *testing.T) {
	// Test standard encoding
	data := []byte("test data")
	encoded := base64.StdEncoding.EncodeToString(data)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, data, decoded)

	// Test URL encoding (no padding)
	urlEncoded := base64.RawURLEncoding.EncodeToString(data)
	urlDecoded, err := base64.RawURLEncoding.DecodeString(urlEncoded)
	require.NoError(t, err)
	assert.Equal(t, data, urlDecoded)
}
