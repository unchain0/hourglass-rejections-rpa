package webauthn

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
)

var execCommand = exec.Command

type Authenticator struct {
	storage    *Storage
	baseURL    string
	httpClient HTTPClient
	xsrfToken  string
	hgLogin    string
}

func NewAuthenticator(storagePath, baseURL string) (*Authenticator, error) {
	storage, err := NewStorage(storagePath)
	if err != nil {
		return nil, err
	}

	return &Authenticator{
		storage:    storage,
		baseURL:    baseURL,
		httpClient: defaultHTTPClient,
	}, nil
}

func (a *Authenticator) SetCookies(xsrfToken, hgLogin string) {
	a.xsrfToken = xsrfToken
	a.hgLogin = hgLogin
}

func (a *Authenticator) Register(userName string) (*Credential, error) {
	beginResp, err := a.beginRegistration(userName)
	if err != nil {
		return nil, fmt.Errorf("begin registration failed: %w", err)
	}

	userID := beginResp.PublicKey.User.ID
	if userID == "" {
		userID = generateUserID()
	}

	rpID := beginResp.PublicKey.Rp.ID
	if rpID == "" {
		rpID = "hourglass-app.com"
	}

	credential, err := GenerateCredential(rpID, userID, userName)
	if err != nil {
		return nil, fmt.Errorf("generate credential failed: %w", err)
	}

	fmt.Printf("DEBUG: Generated credential - ID: %s, UserID: %s, RPID: %s\n",
		credential.ID, credential.UserID, credential.RPID)

	attestation, err := a.createAttestation(credential, beginResp)
	if err != nil {
		return nil, fmt.Errorf("create attestation failed: %w", err)
	}

	fmt.Printf("DEBUG: Attestation - Type: %s, ID: %s, RawID: %s\n",
		attestation.Type, attestation.ID, attestation.RawID)

	if err := a.finishRegistration(attestation); err != nil {
		return nil, fmt.Errorf("finish registration failed: %w", err)
	}

	storedCreds, err := a.storage.Load()
	if err != nil {
		return nil, fmt.Errorf("load credentials failed: %w", err)
	}

	storedCreds.Credentials = append(storedCreds.Credentials, *credential)
	if err := a.storage.Save(storedCreds); err != nil {
		return nil, fmt.Errorf("save credential failed: %w", err)
	}

	return credential, nil
}

type BeginRegistrationResponse struct {
	PublicKey PublicKeyClass `json:"publicKey"`
}

type PublicKeyClass struct {
	Rp                     Rp                     `json:"rp"`
	User                   User                   `json:"user"`
	Challenge              string                 `json:"challenge"`
	PubKeyCredParams       []PubKeyCredParam      `json:"pubKeyCredParams"`
	Timeout                int64                  `json:"timeout"`
	ExcludeCredentials     []ExcludeCredential    `json:"excludeCredentials"`
	AuthenticatorSelection AuthenticatorSelection `json:"authenticatorSelection"`
}

type AuthenticatorSelection struct {
	AuthenticatorAttachment string `json:"authenticatorAttachment"`
}

type ExcludeCredential struct {
	Type Type   `json:"type"`
	ID   string `json:"id"`
}

type PubKeyCredParam struct {
	Type Type  `json:"type"`
	Alg  int64 `json:"alg"`
}

type Rp struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type User struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	ID          string `json:"id"`
}

type Type string

const (
	PublicKey Type = "public-key"
)

func (a *Authenticator) beginRegistration(userName string) (*BeginRegistrationResponse, error) {
	url := fmt.Sprintf("%s/auth/webauthn/register/begin", a.baseURL)

	var body []byte
	var err error

	if a.xsrfToken != "" && a.hgLogin != "" {
		body, err = a.curlGet(url)
	} else {
		body, err = a.httpGet(url)
	}

	if err != nil {
		return nil, err
	}

	var beginResp BeginRegistrationResponse
	if err := json.Unmarshal(body, &beginResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	fmt.Printf("DEBUG: Begin response - Challenge: %s, RP ID: %s, User ID: %s\n",
		beginResp.PublicKey.Challenge,
		beginResp.PublicKey.Rp.ID,
		beginResp.PublicKey.User.ID)

	return &beginResp, nil
}

func (a *Authenticator) curlGet(url string) ([]byte, error) {
	args := []string{
		"-s", "-w", "\n%{http_code}",
		"-H", fmt.Sprintf("X-Hourglass-XSRF-Token: %s", a.xsrfToken),
		"-b", fmt.Sprintf("hglogin=%s", a.hgLogin),
		url,
	}

	cmd := execCommand("curl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("curl failed: %w, output: %s", err, string(output))
	}

	lines := bytes.Split(output, []byte("\n"))
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid curl response")
	}

	statusCode := string(lines[len(lines)-1])
	body := bytes.Join(lines[:len(lines)-1], []byte("\n"))

	if statusCode != "200" {
		return nil, fmt.Errorf("unexpected status: %s, body: %s", statusCode, string(body))
	}

	return body, nil
}

func (a *Authenticator) httpGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

type AttestationResponse struct {
	Type                    string                 `json:"type"`
	ID                      string                 `json:"id"`
	RawID                   string                 `json:"rawId"`
	AuthenticatorAttachment string                 `json:"authenticatorAttachment,omitempty"`
	Response                AttestationData        `json:"response"`
	ClientExtensionResults  map[string]interface{} `json:"clientExtensionResults,omitempty"`
}

type AttestationData struct {
	ClientDataJSON         string                 `json:"clientDataJSON"`
	AttestationObject      string                 `json:"attestationObject"`
	Transports             []string               `json:"transports,omitempty"`
	ClientExtensionResults map[string]interface{} `json:"clientExtensionResults,omitempty"`
}

func (a *Authenticator) createAttestation(cred *Credential, beginResp *BeginRegistrationResponse) (*AttestationResponse, error) {
	challenge := beginResp.PublicKey.Challenge
	origin := fmt.Sprintf("https://app.%s", beginResp.PublicKey.Rp.ID)

	clientData := protocol.CollectedClientData{
		Type:        "webauthn.create",
		Challenge:   challenge,
		Origin:      origin,
		CrossOrigin: false,
	}

	clientDataJSON, err := json.Marshal(clientData)
	if err != nil {
		return nil, fmt.Errorf("marshal client data: %w", err)
	}

	clientDataHash := sha256.Sum256(clientDataJSON)

	authData, err := a.createAuthenticatorData(cred, clientDataHash[:])
	if err != nil {
		return nil, fmt.Errorf("create authenticator data: %w", err)
	}

	debugAuthData(authData)

	attestationObject, err := a.createAttestationObject(authData)
	if err != nil {
		return nil, fmt.Errorf("create attestation object: %w", err)
	}

	return &AttestationResponse{
		Type:                    "public-key",
		ID:                      cred.ID,
		RawID:                   cred.ID,
		AuthenticatorAttachment: "platform",
		Response: AttestationData{
			ClientDataJSON:    base64.RawURLEncoding.EncodeToString(clientDataJSON),
			AttestationObject: base64.RawURLEncoding.EncodeToString(attestationObject),
			Transports:        []string{"internal", "hybrid"},
		},
		ClientExtensionResults: map[string]interface{}{},
	}, nil
}

func (a *Authenticator) createAuthenticatorData(cred *Credential, clientDataHash []byte) ([]byte, error) {
	rpIDHash := sha256.Sum256([]byte(cred.RPID))

	flags := byte(0x41)

	signCount := make([]byte, 4)
	binary.BigEndian.PutUint32(signCount, cred.SignCount)

	credIDBytes, err := cred.GetCredentialIDBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get credential ID bytes: %w", err)
	}
	fmt.Printf("DEBUG: credential ID bytes length: %d\n", len(credIDBytes))

	credIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(credIDLen, uint16(len(credIDBytes)))

	aaguid := make([]byte, 16)

	authData := make([]byte, 0, 200)
	authData = append(authData, rpIDHash[:]...)
	authData = append(authData, flags)
	authData = append(authData, signCount...)
	authData = append(authData, aaguid...)
	authData = append(authData, credIDLen...)
	authData = append(authData, credIDBytes...)
	authData = append(authData, cred.PublicKey...)

	return authData, nil
}

func (a *Authenticator) createAttestationObject(authData []byte) ([]byte, error) {
	attObj := protocol.AttestationObject{
		Format:       "none",
		RawAuthData:  authData,
		AttStatement: map[string]any{},
	}

	return webauthncbor.Marshal(attObj)
}

func debugAuthData(authData []byte) {
	if len(authData) < 37 {
		fmt.Printf("DEBUG: authData too short: %d bytes\n", len(authData))
		return
	}
	rpIDHash := authData[:32]
	flags := authData[32]
	signCount := binary.BigEndian.Uint32(authData[33:37])
	fmt.Printf("DEBUG: rpIdHash: %x...\n", rpIDHash[:8])
	fmt.Printf("DEBUG: flags: %02x\n", flags)
	fmt.Printf("DEBUG: signCount: %d\n", signCount)

	if flags&0x40 != 0 {
		if len(authData) < 55 {
			fmt.Printf("DEBUG: authData too short for credential data: %d bytes\n", len(authData))
			return
		}
		credIDLen := binary.BigEndian.Uint16(authData[53:55])
		fmt.Printf("DEBUG: credentialIdLength: %d\n", credIDLen)
		if len(authData) >= int(55+credIDLen) {
			credID := authData[55 : 55+credIDLen]
			fmt.Printf("DEBUG: credentialId: %s\n", base64.RawURLEncoding.EncodeToString(credID))
		}
	}
}

func (a *Authenticator) finishRegistration(attestation *AttestationResponse) error {
	url := fmt.Sprintf("%s/auth/webauthn/register/finish", a.baseURL)

	body, err := json.Marshal(attestation)
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}

	fmt.Printf("DEBUG: Finish registration payload:\n%s\n\n", string(body))

	var respBody []byte
	if a.xsrfToken != "" && a.hgLogin != "" {
		respBody, err = a.curlPost(url, body)
	} else {
		respBody, err = a.httpPost(url, body)
	}

	if err != nil {
		return err
	}

	if len(respBody) > 0 {
		return nil
	}

	return nil
}

func (a *Authenticator) curlPost(url string, data []byte) ([]byte, error) {
	args := []string{
		"-s", "-w", "\n%{http_code}",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", fmt.Sprintf("X-Hourglass-XSRF-Token: %s", a.xsrfToken),
		"-H", "Origin: https://app.hourglass-app.com",
		"-H", "Referer: https://app.hourglass-app.com/v2/page/app/user/auth/1828629",
		"-b", fmt.Sprintf("hglogin=%s", a.hgLogin),
		"-d", string(data),
		url,
	}

	cmd := execCommand("curl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("curl failed: %w, output: %s", err, string(output))
	}

	lines := bytes.Split(output, []byte("\n"))
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid curl response")
	}

	statusCode := string(lines[len(lines)-1])
	body := bytes.Join(lines[:len(lines)-1], []byte("\n"))

	fmt.Printf("DEBUG: Response status: %s\n", statusCode)
	fmt.Printf("DEBUG: Response body: %s\n", string(body))

	if statusCode != "200" && statusCode != "201" {
		return nil, fmt.Errorf("registration failed: status=%s, body=%s", statusCode, string(body))
	}

	return body, nil
}

func (a *Authenticator) httpPost(url string, data []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func generateUserID() string {
	id := make([]byte, 16)
	rand.Read(id)
	return base64.StdEncoding.EncodeToString(id)
}
