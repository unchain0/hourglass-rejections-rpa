// Package setup-auth provides interactive authentication setup for Hourglass RPA.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

var (
	chromeStatFn           = os.Stat
	prepareChromeProfileFn = webauthn.PrepareChromeProfile
	execCommandFn          = exec.Command
)

const (
	defaultBaseURL    = "https://app.hourglass-app.com"
	defaultConfigDir  = ".hourglass-rpa"
	defaultTokensFile = "auth-tokens.json"
)

// FileSystem abstracts filesystem operations used by setup.
type FileSystem interface {
	UserHomeDir() (string, error)
	MkdirAll(path string, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
}

// BrowserAuthFactory creates browser-based authenticators.
type BrowserAuthFactory interface {
	NewBrowserAuth(baseURL string) browserAuth
}

type browserAuth interface {
	Authenticate() (*webauthn.AuthTokens, error)
	ExtractTokensFromProfile() (*webauthn.AuthTokens, error)
	WithHeadless(headless bool) browserAuth
	WithProfileDir(profileDir string) browserAuth
}

type credentialRegistrar interface {
	SetCookies(xsrfToken, hgLogin string)
	Register(userName string) (*webauthn.Credential, error)
}

// UserInput abstracts interactive console input.
type UserInput interface {
	Confirm(prompt string) (bool, error)
	ReadLine() (string, error)
}

// SCPClient copies local files to a remote host over SCP.
type SCPClient interface {
	CopyFile(localPath, remoteHost, remotePath string) error
}

type osFileSystem struct{}

func (osFileSystem) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (osFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (osFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

type browserAuthAdapter struct {
	auth               *webauthn.BrowserAuth
	authenticateFunc   func() (*webauthn.AuthTokens, error)
	extractTokensFunc  func() (*webauthn.AuthTokens, error)
	withHeadlessFunc   func(bool) *webauthn.BrowserAuth
	withProfileDirFunc func(string) *webauthn.BrowserAuth
}

func (b *browserAuthAdapter) Authenticate() (*webauthn.AuthTokens, error) {
	if b != nil && b.authenticateFunc != nil {
		return b.authenticateFunc()
	}
	if b == nil || b.auth == nil {
		return nil, errors.New("browser auth is not configured")
	}

	return b.auth.Authenticate()
}

func (b *browserAuthAdapter) ExtractTokensFromProfile() (*webauthn.AuthTokens, error) {
	if b != nil && b.extractTokensFunc != nil {
		return b.extractTokensFunc()
	}
	if b == nil || b.auth == nil {
		return nil, errors.New("browser auth is not configured")
	}

	return b.auth.ExtractTokensFromProfile()
}

func (b *browserAuthAdapter) WithHeadless(headless bool) browserAuth {
	if b != nil && b.withHeadlessFunc != nil {
		b.auth = b.withHeadlessFunc(headless)
		return b
	}
	if b == nil || b.auth == nil {
		return b
	}

	b.auth = b.auth.WithHeadless(headless)
	return b
}

func (b *browserAuthAdapter) WithProfileDir(profileDir string) browserAuth {
	if b != nil && b.withProfileDirFunc != nil {
		b.auth = b.withProfileDirFunc(profileDir)
		return b
	}
	if b == nil || b.auth == nil {
		return b
	}

	b.auth = b.auth.WithProfileDir(profileDir)
	return b
}

type webauthnBrowserAuthFactory struct{}

func (webauthnBrowserAuthFactory) NewBrowserAuth(baseURL string) browserAuth {
	return &browserAuthAdapter{auth: webauthn.NewBrowserAuth(baseURL)}
}

var newBrowserAuth = func(baseURL string) browserAuth {
	return webauthnBrowserAuthFactory{}.NewBrowserAuth(baseURL)
}

type functionBrowserAuthFactory struct {
	newFn func(string) browserAuth
}

func (f functionBrowserAuthFactory) NewBrowserAuth(baseURL string) browserAuth {
	return f.newFn(baseURL)
}

type consoleUserInput struct {
	reader *bufio.Reader
}

func newConsoleUserInput(reader io.Reader) *consoleUserInput {
	return &consoleUserInput{reader: bufio.NewReader(reader)}
}

func (c *consoleUserInput) Confirm(prompt string) (bool, error) {
	fmt.Print(prompt)
	input, err := c.ReadLine()
	if err != nil {
		return false, err
	}

	return strings.EqualFold(strings.TrimSpace(input), "yes"), nil
}

func (c *consoleUserInput) ReadLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}

	return strings.TrimSpace(line), nil
}

type execSCPClient struct {
	stdout io.Writer
	stderr io.Writer
}

func (c *execSCPClient) CopyFile(localPath, remoteHost, remotePath string) error {
	cmd := exec.Command("scp", "-p", localPath, fmt.Sprintf("%s:%s", remoteHost, remotePath))
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to transfer tokens: %w", err)
	}

	return nil
}

type setupRunner struct {
	fs              FileSystem
	browserAuthFact BrowserAuthFactory
	authFactory     func(storagePath, baseURL string) (credentialRegistrar, error)
	launchBrowser   func(profileDir, loginURL string) error
	waitForConfirm  func() error
	userInput       UserInput
	scpClient       SCPClient
	baseURL         string
	configDir       string
	tokensFile      string
	osExit          func(int)
	getenv          func(string) string
}

func newSetupRunner() *setupRunner {
	baseURL := os.Getenv("HOURGLASS_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &setupRunner{
		fs:              osFileSystem{},
		browserAuthFact: functionBrowserAuthFactory{newFn: newBrowserAuth},
		authFactory:     defaultCredentialRegistrarFactory,
		userInput:       newConsoleUserInput(os.Stdin),
		scpClient: &execSCPClient{
			stdout: os.Stdout,
			stderr: os.Stderr,
		},
		baseURL:    baseURL,
		configDir:  defaultConfigDir,
		tokensFile: defaultTokensFile,
		osExit:     os.Exit,
		getenv:     os.Getenv,
	}
}

var defaultCredentialRegistrarFactory = func(storagePath, baseURL string) (credentialRegistrar, error) {
	return webauthn.NewAuthenticator(storagePath, baseURL)
}

type setupOptions struct {
	getenv        func(string) string
	osUserHomeDir func() (string, error)
}

type optionsFileSystem struct {
	base          FileSystem
	userHomeDirFn func() (string, error)
}

func (o *optionsFileSystem) UserHomeDir() (string, error) {
	if o.userHomeDirFn != nil {
		return o.userHomeDirFn()
	}

	return o.base.UserHomeDir()
}

func (o *optionsFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return o.base.MkdirAll(path, perm)
}

func (o *optionsFileSystem) ReadFile(path string) ([]byte, error) {
	return o.base.ReadFile(path)
}

func (o *optionsFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return o.base.WriteFile(path, data, perm)
}

func main() {
	_ = godotenv.Load()
	runner := newSetupRunner()
	runner.launchBrowser = launchChromeForManualLogin
	runner.waitForConfirm = waitForBrowserConfirmation
	if err := runner.run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Setup failed: %v\n", err)
		runner.osExit(1)
	}
}

func run(opts setupOptions) error {
	runner := newSetupRunner()
	if opts.getenv != nil {
		runner.getenv = opts.getenv
	}
	runner.fs = &optionsFileSystem{
		base:          runner.fs,
		userHomeDirFn: opts.osUserHomeDir,
	}

	return runner.run()
}

func (r *setupRunner) run() error {
	fmt.Println("🔐 Hourglass Rejections RPA - Authentication Setup")
	fmt.Println("============================================")
	fmt.Println()

	homeDir, err := r.fs.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, r.configDir)
	tokensPath := filepath.Join(configDir, r.tokensFile)
	credentialsPath := filepath.Join(configDir, "webauthn-credentials.json")
	profileDir := r.chromeProfileDir(configDir)

	fmt.Println("📍 Configuration Directory:", configDir)
	fmt.Println("📄 Tokens File:        ", tokensPath)
	fmt.Println("🔐 WebAuthn File:      ", credentialsPath)
	fmt.Println("🌐 Chrome Profile:     ", profileDir)
	fmt.Println()

	if err := r.fs.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := r.fs.MkdirAll(profileDir, 0700); err != nil {
		return fmt.Errorf("failed to create chrome profile directory: %w", err)
	}

	existingTokens, err := r.checkExistingTokens(tokensPath)
	if err != nil {
		return fmt.Errorf("failed to check existing tokens: %w", err)
	}

	if environmentTokens := r.environmentBootstrapTokens(credentialsPath); environmentTokens != nil {
		return r.bootstrapEnvironmentSession(tokensPath, credentialsPath, environmentTokens)
	}

	if existingTokens != nil {
		if !existingTokens.IsExpired() {
			fmt.Println("✅ Valid tokens found!")
			fmt.Printf("   ⏰ Expires: %s\n", existingTokens.ExpiresAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("   ⏳ Time remaining: %s\n", time.Until(existingTokens.ExpiresAt).Round(time.Minute))

			reauth, err := r.userInput.Confirm("\n🔄 Re-authenticate anyway? (yes/no): ")
			if err != nil {
				return fmt.Errorf("failed to read re-authentication confirmation: %w", err)
			}
			if !reauth {
				fmt.Println("\n✅ Using existing tokens.")
				return nil
			}
		} else {
			fmt.Println("⚠️  Existing tokens have expired.")
			fmt.Printf("   ⏰ Expired at: %s\n", existingTokens.ExpiresAt.Format("2006-01-02 15:04:05"))
			fmt.Println()
		}
	}

	fmt.Println("🌐 Starting browser authentication...")
	fmt.Println("📌 A normal Chrome window will open with the persistent profile - complete the login manually.")
	fmt.Println()

	loginURL := strings.TrimSuffix(r.baseURL, "/") + "/v2/page/app"
	if r.launchBrowser != nil {
		if err := r.launchBrowser(profileDir, loginURL); err != nil {
			return fmt.Errorf("failed to launch manual browser: %w", err)
		}
	}
	if r.waitForConfirm != nil {
		if err := r.waitForConfirm(); err != nil {
			return fmt.Errorf("manual browser confirmation failed: %w", err)
		}
	}

	authenticator := r.browserAuthFact.NewBrowserAuth(r.baseURL).WithHeadless(false).WithProfileDir(profileDir)
	tokens, err := authenticator.ExtractTokensFromProfile()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("\n✅ Authentication successful!")
	fmt.Printf("   🔑 HGLogin Token:  %s...\n", r.truncate(tokens.HGLogin, 30))
	fmt.Printf("   🔒 XSRF Token:     %s...\n", r.truncate(tokens.XSRFToken, 30))
	fmt.Printf("   ⏰ Expires At:      %s\n", tokens.ExpiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   ⏳ Valid for:       %s\n", time.Until(tokens.ExpiresAt).Round(time.Minute))
	fmt.Println()

	if err := r.saveTokens(tokensPath, tokens); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	if err := r.registerWebAuthnCredential(credentialsPath, tokens); err != nil {
		fmt.Println("⚠️  WebAuthn registration skipped.")
		fmt.Printf("   Reason: %v\n", err)
		fmt.Println("   ✅ Persistent Chrome profile bootstrap is still valid for token refresh using CHROME_PROFILE_DIR.")
		fmt.Println()
	}

	fmt.Println("💾 Tokens saved successfully!")
	fmt.Printf("   📁 Location: %s\n", tokensPath)
	fmt.Println()

	return r.askVPSUploadWithCredentials(tokensPath, credentialsPath)
}

func (r *setupRunner) environmentBootstrapTokens(credentialsPath string) *webauthn.AuthTokens {
	if r.getenv == nil {
		return nil
	}

	hgLogin := r.getenv("HOURGLASS_HGLOGIN_COOKIE")
	xsrfToken := r.getenv("HOURGLASS_XSRF_TOKEN")
	if hgLogin == "" || xsrfToken == "" {
		return nil
	}
	if _, err := os.Stat(credentialsPath); !errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return &webauthn.AuthTokens{
		HGLogin:   hgLogin,
		XSRFToken: xsrfToken,
		ExpiresAt: time.Now().Add(-time.Second),
	}
}

func (r *setupRunner) bootstrapEnvironmentSession(tokensPath, credentialsPath string, tokens *webauthn.AuthTokens) error {
	fmt.Println("🔑 Valid session found in the environment; registering automatic renewal without opening a browser.")
	if err := r.registerWebAuthnCredential(credentialsPath, tokens); err != nil {
		return fmt.Errorf("failed to bootstrap automatic renewal from environment session: %w", err)
	}
	if err := r.saveTokens(tokensPath, tokens); err != nil {
		return fmt.Errorf("failed to save bootstrap tokens: %w", err)
	}

	fmt.Println("✅ WebAuthn credential registered. The token-refresh command can now renew the session automatically.")
	return r.askVPSUploadWithCredentials(tokensPath, credentialsPath)
}

func (r *setupRunner) chromeProfileDir(configDir string) string {
	if profileDir := os.Getenv("CHROME_PROFILE_DIR"); profileDir != "" {
		return profileDir
	}

	return filepath.Join(configDir, "chrome-profile")
}

func launchChromeForManualLogin(profileDir, loginURL string) error {
	chromePath := os.Getenv("CHROME_BIN")
	if chromePath == "" {
		chromePath = os.Getenv("CHROME_PATH")
	}
	if chromePath == "" {
		for _, candidate := range []string{"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium-browser", "/usr/bin/chromium"} {
			if _, err := chromeStatFn(candidate); err == nil {
				chromePath = candidate
				break
			}
		}
	}
	if chromePath == "" {
		return fmt.Errorf("chrome/chromium not found: set CHROME_BIN environment variable or install Chrome")
	}

	if err := prepareChromeProfileFn(profileDir); err != nil {
		return err
	}

	cmd := execCommandFn(chromePath,
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--new-window",
		"--no-first-run",
		"--no-default-browser-check",
		loginURL,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	return nil
}

func waitForBrowserConfirmation() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("🔐 O Chrome normal foi aberto com o perfil persistente.")
	fmt.Println("👉 Faça o login manualmente no Google/Hourglass, confirme que chegou no app e feche a janela do Chrome.")
	fmt.Print("⏎ Depois disso, pressione Enter para extrair os tokens do perfil salvo... ")
	_, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation input: %w", err)
	}
	return nil
}

func checkExistingTokens(path string) (*webauthn.AuthTokens, error) {
	runner := newSetupRunner()
	return runner.checkExistingTokens(path)
}

func (r *setupRunner) checkExistingTokens(path string) (*webauthn.AuthTokens, error) {
	data, err := r.fs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read tokens file: %w", err)
	}

	var tokens webauthn.AuthTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("failed to parse tokens file: %w", err)
	}

	return &tokens, nil
}

func saveTokens(path string, tokens *webauthn.AuthTokens) error {
	runner := newSetupRunner()
	return runner.saveTokens(path, tokens)
}

func (r *setupRunner) saveTokens(path string, tokens *webauthn.AuthTokens) error {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	if err := r.fs.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write tokens file: %w", err)
	}

	return nil
}

func (r *setupRunner) registerWebAuthnCredential(credentialsPath string, tokens *webauthn.AuthTokens) error {
	if r.authFactory == nil {
		r.authFactory = defaultCredentialRegistrarFactory
	}

	fmt.Println("🔐 Registering WebAuthn credential for automatic renewal...")

	authenticator, err := r.authFactory(credentialsPath, r.baseURL)
	if err != nil {
		return fmt.Errorf("failed to create authenticator: %w", err)
	}

	authenticator.SetCookies(tokens.XSRFToken, tokens.HGLogin)
	if _, err := authenticator.Register("Hourglass RPA"); err != nil {
		return fmt.Errorf("failed to register credential: %w", err)
	}

	fmt.Printf("   📁 Credential stored at: %s\n", credentialsPath)
	fmt.Println()

	return nil
}

func askVPSUpload(tokensPath string) error {
	runner := newSetupRunner()
	return runner.askVPSUpload(tokensPath)
}

func (r *setupRunner) askVPSUpload(tokensPath string) error {
	fmt.Println("📦 VPS Deployment")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("You can copy the authentication tokens to your VPS for remote deployment.")
	fmt.Println()

	confirm, err := r.userInput.Confirm("📡 Transfer tokens to VPS via SCP? (yes/no): ")
	if err != nil {
		return fmt.Errorf("failed to read transfer confirmation: %w", err)
	}

	if !confirm {
		fmt.Println("\n✅ Setup complete!")
		return nil
	}

	fmt.Println()
	fmt.Print("🖥️  VPS host (user@host): ")
	vpsHost, err := r.userInput.ReadLine()
	if err != nil {
		return fmt.Errorf("failed to read VPS host: %w", err)
	}

	if vpsHost == "" {
		fmt.Println("❌ VPS host cannot be empty")
		return nil
	}

	fmt.Print("📂 VPS target path (default: ~/.hourglass-rpa/auth-tokens.json): ")
	vpsPath, err := r.userInput.ReadLine()
	if err != nil {
		return fmt.Errorf("failed to read VPS target path: %w", err)
	}

	if vpsPath == "" {
		vpsPath = "~/.hourglass-rpa/auth-tokens.json"
	}

	fmt.Println()
	fmt.Println("📤 Transferring tokens to VPS...")

	if err := r.scpClient.CopyFile(tokensPath, vpsHost, vpsPath); err != nil {
		return err
	}

	fmt.Println("\n✅ Tokens transferred successfully!")
	fmt.Printf("   🖥️  VPS Host: %s\n", vpsHost)
	fmt.Printf("   📂 Target:   %s\n", vpsPath)
	fmt.Println()
	fmt.Println("📋 Next steps:")
	fmt.Println("1. SSH into your VPS")
	fmt.Println("2. Verify tokens are in the correct location")
	fmt.Println("3. Ensure WEBAUTHN_TOKENS_PATH environment variable is set (if needed)")
	fmt.Println("4. Run the application: ./rpa")
	fmt.Println()
	fmt.Println("✅ Setup complete!")

	return nil
}

func (r *setupRunner) askVPSUploadWithCredentials(tokensPath, credentialsPath string) error {
	fmt.Println("📦 VPS Deployment")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("You can copy the authentication files to your VPS for remote deployment.")
	fmt.Println()

	confirm, err := r.userInput.Confirm("📡 Transfer authentication files to VPS via SCP? (yes/no): ")
	if err != nil {
		return fmt.Errorf("failed to read transfer confirmation: %w", err)
	}

	if !confirm {
		fmt.Println("\n✅ Setup complete!")
		return nil
	}

	fmt.Println()
	fmt.Print("🖥️  VPS host (user@host): ")
	vpsHost, err := r.userInput.ReadLine()
	if err != nil {
		return fmt.Errorf("failed to read VPS host: %w", err)
	}

	if vpsHost == "" {
		fmt.Println("❌ VPS host cannot be empty")
		return nil
	}

	fmt.Print("📂 VPS target path for tokens (default: ~/.hourglass-rpa/auth-tokens.json): ")
	vpsTokensPath, err := r.userInput.ReadLine()
	if err != nil {
		return fmt.Errorf("failed to read VPS target path: %w", err)
	}

	if vpsTokensPath == "" {
		vpsTokensPath = "~/.hourglass-rpa/auth-tokens.json"
	}

	vpsCredentialsPath := path.Join(path.Dir(vpsTokensPath), "webauthn-credentials.json")

	fmt.Println()
	fmt.Println("📤 Transferring token file...")
	if err := r.scpClient.CopyFile(tokensPath, vpsHost, vpsTokensPath); err != nil {
		return err
	}

	fmt.Println("📤 Transferring WebAuthn credential...")
	if err := r.scpClient.CopyFile(credentialsPath, vpsHost, vpsCredentialsPath); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Authentication files transferred successfully!")
	fmt.Printf("   📄 Tokens:      %s:%s\n", vpsHost, vpsTokensPath)
	fmt.Printf("   🔐 Credentials: %s:%s\n", vpsHost, vpsCredentialsPath)
	fmt.Println()
	fmt.Println("📋 Next steps:")
	fmt.Println("1. SSH into your VPS")
	fmt.Println("2. Verify both files are in ~/.hourglass-rpa")
	fmt.Println("3. Start the application: ./rpa")
	fmt.Println()
	fmt.Println("✅ Setup complete!")

	return nil
}

func truncate(s string, maxLen int) string {
	runner := newSetupRunner()
	return runner.truncate(s, maxLen)
}

func (r *setupRunner) truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
