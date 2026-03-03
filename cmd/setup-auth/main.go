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
	"path/filepath"
	"strings"
	"time"

	"hourglass-rejections-rpa/internal/auth/webauthn"
)

const (
	defaultBaseURL    = "https://app.hourglass-app.com/api/v0.2"
	defaultConfigDir  = ".hourglass-rpa"
	defaultTokensFile = "auth-tokens.json"
)

type FileSystem interface {
	UserHomeDir() (string, error)
	MkdirAll(path string, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
}

type BrowserAuthFactory interface {
	NewBrowserAuth(baseURL string) browserAuth
}

type browserAuth interface {
	Authenticate() (*webauthn.AuthTokens, error)
	WithHeadless(headless bool) browserAuth
}

type UserInput interface {
	Confirm(prompt string) (bool, error)
	ReadLine() (string, error)
}

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
	auth *webauthn.BrowserAuth
}

func (b *browserAuthAdapter) Authenticate() (*webauthn.AuthTokens, error) {
	return b.auth.Authenticate()
}

func (b *browserAuthAdapter) WithHeadless(headless bool) browserAuth {
	b.auth = b.auth.WithHeadless(headless)
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
	userInput       UserInput
	scpClient       SCPClient
	baseURL         string
	configDir       string
	tokensFile      string
	osExit          func(int)
}

func newSetupRunner() *setupRunner {
	return &setupRunner{
		fs:              osFileSystem{},
		browserAuthFact: functionBrowserAuthFactory{newFn: newBrowserAuth},
		userInput:       newConsoleUserInput(os.Stdin),
		scpClient: &execSCPClient{
			stdout: os.Stdout,
			stderr: os.Stderr,
		},
		baseURL:    defaultBaseURL,
		configDir:  defaultConfigDir,
		tokensFile: defaultTokensFile,
		osExit:     os.Exit,
	}
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
	runner := newSetupRunner()
	if err := runner.run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Setup failed: %v\n", err)
		runner.osExit(1)
	}
}

func run(opts setupOptions) error {
	runner := newSetupRunner()
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

	fmt.Println("📍 Configuration Directory:", configDir)
	fmt.Println("📄 Tokens File:        ", tokensPath)
	fmt.Println()

	if err := r.fs.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	existingTokens, err := r.checkExistingTokens(tokensPath)
	if err != nil {
		return fmt.Errorf("failed to check existing tokens: %w", err)
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
	fmt.Println("📌 A Chrome window will open - please complete the login process.")
	fmt.Println()

	authenticator := r.browserAuthFact.NewBrowserAuth(r.baseURL)
	tokens, err := authenticator.Authenticate()
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

	fmt.Println("💾 Tokens saved successfully!")
	fmt.Printf("   📁 Location: %s\n", tokensPath)
	fmt.Println()

	return r.askVPSUpload(tokensPath)
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
