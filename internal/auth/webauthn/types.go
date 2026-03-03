package webauthn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fxamacker/cbor/v2"
)

type Credential struct {
	ID         string    `json:"id"`
	PrivateKey []byte    `json:"private_key"`
	PublicKey  []byte    `json:"public_key"`
	UserID     string    `json:"user_id"`
	UserName   string    `json:"user_name"`
	RPID       string    `json:"rp_id"`
	SignCount  uint32    `json:"sign_count"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

type StoredCredentials struct {
	Version     int          `json:"version"`
	Credentials []Credential `json:"credentials"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Storage struct {
	path string
}

func NewStorage(path string) (*Storage, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create credentials directory: %w", err)
	}

	return &Storage{path: path}, nil
}

func (s *Storage) Load() (*StoredCredentials, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &StoredCredentials{
				Version:     1,
				Credentials: []Credential{},
				UpdatedAt:   time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds StoredCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

func (s *Storage) Save(creds *StoredCredentials) error {
	creds.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	return os.WriteFile(s.path, data, 0600)
}

func GenerateCredential(rpID, userID, userName string) (*Credential, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	credentialID := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, credentialID); err != nil {
		return nil, fmt.Errorf("failed to generate credential ID: %w", err)
	}

	publicKeyBytes, err := createCOSEPublicKey(privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create COSE public key: %w", err)
	}

	return &Credential{
		ID:         base64.RawURLEncoding.EncodeToString(credentialID),
		PrivateKey: privateKeyBytes,
		PublicKey:  publicKeyBytes,
		UserID:     userID,
		UserName:   userName,
		RPID:       rpID,
		SignCount:  0,
		CreatedAt:  time.Now(),
	}, nil
}

func LoadCredentialFromPEM(pemPath, credentialID, rpID, userID, userName string) (*Credential, error) {
	pemData, err := os.ReadFile(pemPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PEM file: %w", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	ecdsaKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ECDSA")
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	publicKeyBytes, err := createCOSEPublicKey(ecdsaKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create COSE public key: %w", err)
	}

	return &Credential{
		ID:         credentialID,
		PrivateKey: privateKeyBytes,
		PublicKey:  publicKeyBytes,
		UserID:     userID,
		UserName:   userName,
		RPID:       rpID,
		SignCount:  0,
		CreatedAt:  time.Now(),
	}, nil
}

func createCOSEPublicKey(pubKey ecdsa.PublicKey) ([]byte, error) {
	xBytes := pubKey.X.Bytes()
	yBytes := pubKey.Y.Bytes()

	xPadded := make([]byte, 32)
	yPadded := make([]byte, 32)
	copy(xPadded[32-len(xBytes):], xBytes)
	copy(yPadded[32-len(yBytes):], yBytes)

	type COSEKey struct {
		Kty int64  `cbor:"1,keyasint"`
		Alg int64  `cbor:"3,keyasint"`
		Crv int64  `cbor:"-1,keyasint"`
		X   []byte `cbor:"-2,keyasint"`
		Y   []byte `cbor:"-3,keyasint"`
	}

	coseKey := COSEKey{
		Kty: 2,
		Alg: -7,
		Crv: 1,
		X:   xPadded,
		Y:   yPadded,
	}

	return cbor.Marshal(coseKey)
}

func (c *Credential) GetPrivateKey() (*ecdsa.PrivateKey, error) {
	key, err := x509.ParsePKCS8PrivateKey(c.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ECDSA")
	}

	return ecdsaKey, nil
}

func (c *Credential) GetCredentialIDBytes() ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(c.ID)
}

func (c *Credential) Sign(challenge, clientDataHash []byte) ([]byte, error) {
	privateKey, err := c.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("get private key: %w", err)
	}

	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, clientDataHash[:])
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	c.incrementSignCount()

	return signature, nil
}

func (c *Credential) GetUserIDBytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(c.UserID)
}

func (c *Credential) incrementSignCount() {
	c.SignCount++
	c.LastUsedAt = time.Now()
}

type AuthTokens struct {
	HGLogin   string    `json:"hg_login"`
	XSRFToken string    `json:"xsrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (t *AuthTokens) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

func (t *AuthTokens) IsNearExpiry(threshold time.Duration) bool {
	return time.Until(t.ExpiresAt) < threshold
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

type rawHeadersTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t *rawHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header[k] = []string{v}
	}
	return t.base.RoundTrip(req)
}

func newClientWithRawHeaders(headers map[string]string) *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &rawHeadersTransport{headers: headers, base: http.DefaultTransport},
	}
}
