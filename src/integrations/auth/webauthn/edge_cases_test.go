package webauthn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAssertionRejectsInvalidUserHandle(t *testing.T) {
	cred := mustGenerateCredentialForTest(t)
	cred.UserID = "%%%invalid%%%"

	_, err := (&Authenticator{}).createAssertion(cred, testBeginAuthenticationResponse())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode user handle")
}

func TestCreateAttestationObjectReportsEncoderError(t *testing.T) {
	original := newCTAP2EncMode
	t.Cleanup(func() { newCTAP2EncMode = original })
	newCTAP2EncMode = func() (cbor.EncMode, error) {
		return nil, errors.New("encoder failed")
	}

	_, err := (&Authenticator{}).createAttestationObject([]byte("auth-data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create CTAP2 CBOR encoder")
}

func TestCreateCOSEPublicKeyRejectsInvalidKeys(t *testing.T) {
	t.Run("missing curve", func(t *testing.T) {
		_, err := createCOSEPublicKey(ecdsa.PublicKey{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encode ECDSA public key")
	})

	t.Run("non P-256 key", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		require.NoError(t, err)

		_, err = createCOSEPublicKey(privateKey.PublicKey)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected ECDSA public key length")
	})
}

func TestChromeVersionEdgeCases(t *testing.T) {
	restoreBrowserAuthHooks(t)
	chromeVersionOutput = func(string) ([]byte, error) {
		return []byte("Chromium unknown"), nil
	}

	_, err := validateChromeVersion("chromium")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse browser version")

	_, err = chromeMajorVersion("Chromium " + strings.Repeat("9", 100))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse browser version")
}

func TestDefaultProcessExistsRecognizesCurrentProcess(t *testing.T) {
	assert.True(t, processExists(os.Getpid()))
}

func TestBrowserProfileCookieStoreEdgeCases(t *testing.T) {
	assert.False(t, browserProfileHasCookieStore(filepath.Join(t.TempDir(), "missing")))

	profileDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "not-a-profile"), []byte("file"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(profileDir, "Other"), 0o700))
	assert.False(t, browserProfileHasCookieStore(profileDir))
}

func TestBrowserFailureWithoutCredentialFallsBackToWebAuthn(t *testing.T) {
	restoreTokenManagerHooks(t)
	t.Setenv("DISPLAY", ":1")
	browserAuthenticate = func(*BrowserAuth) (*AuthTokens, error) {
		return nil, errors.New("browser failed")
	}

	storage, err := NewStorage(filepath.Join(t.TempDir(), "credentials.json"))
	require.NoError(t, err)
	tm := &TokenManager{
		authenticator: &Authenticator{storage: storage},
		browserAuth:   NewBrowserAuth("https://example.com"),
		storagePath:   storage.path,
	}

	tokens, err := tm.authenticateWithCurrentTokens(&AuthTokens{
		HGLogin:   "current-hg",
		XSRFToken: "current-xsrf",
		ExpiresAt: time.Now(),
	})
	require.Error(t, err)
	assert.Nil(t, tokens)
	assert.Contains(t, err.Error(), "no WebAuthn credentials")
}
