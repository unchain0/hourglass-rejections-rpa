package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hourglass-rejections-rpa/internal/auth/webauthn"
)

var osExit = os.Exit

type registrationRunner struct {
	userHomeDir  func() (string, error)
	mkdirAll     func(path string, perm os.FileMode) error
	consoleInput func(prompt string) (string, error)
	confirm      func(prompt string) (bool, error)
}

func newRegistrationRunner() *registrationRunner {
	reader := bufio.NewReader(os.Stdin)
	return &registrationRunner{
		userHomeDir: os.UserHomeDir,
		mkdirAll:    os.MkdirAll,
		consoleInput: func(prompt string) (string, error) {
			fmt.Print(prompt)
			return reader.ReadString('\n')
		},
		confirm: func(prompt string) (bool, error) {
			fmt.Print(prompt)
			input, err := reader.ReadString('\n')
			if err != nil {
				return false, err
			}
			return strings.EqualFold(strings.TrimSpace(input), "yes"), nil
		},
	}
}

func main() {
	runner := newRegistrationRunner()
	if err := runner.run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Registration failed: %v\n", err)
		osExit(1)
	}
}

func (r *registrationRunner) run() error {
	fmt.Println("🔐 Hourglass WebAuthn Registration")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("This tool registers a WebAuthn credential for automatic authentication.")
	fmt.Println("You only need to run this ONCE per environment.")
	fmt.Println()

	homeDir, err := r.userHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".hourglass-rpa")
	credsPath := filepath.Join(configDir, "webauthn-credentials.json")

	fmt.Printf("📍 Configuration directory: %s\n", configDir)
	fmt.Printf("📄 Credentials file: %s\n", credsPath)
	fmt.Println()

	if err := r.mkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if _, err := os.Stat(credsPath); err == nil {
		overwrite, err := r.confirm("⚠️  Credentials already exist. Overwrite? (yes/no): ")
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if !overwrite {
			fmt.Println("\n✅ Keeping existing credentials.")
			fmt.Println("To re-register, delete the credentials file and run again.")
			return nil
		}
		fmt.Println()
	}

	username, err := r.getUsername()
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("🌐 Starting WebAuthn registration...")
	fmt.Println("📌 A Chrome window will open for authentication.")
	fmt.Println("   You may need to:")
	fmt.Println("   1. Log in to Hourglass (if not already logged in)")
	fmt.Println("   2. Touch your security key / use biometric auth when prompted")
	fmt.Println()

	creds, err := r.performRegistration(username, credsPath)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	r.printSuccess(creds, credsPath)
	return nil
}

func (r *registrationRunner) getUsername() (string, error) {
	for {
		username, err := r.consoleInput("👤 Enter your Hourglass username (email): ")
		if err != nil {
			return "", fmt.Errorf("failed to read username: %w", err)
		}

		username = strings.TrimSpace(username)
		if username == "" {
			fmt.Println("❌ Username cannot be empty")
			continue
		}

		if !strings.Contains(username, "@") {
			fmt.Println("⚠️  Warning: username doesn't look like an email address")
			confirm, err := r.confirm("Continue anyway? (yes/no): ")
			if err != nil {
				return "", err
			}
			if !confirm {
				continue
			}
		}

		return username, nil
	}
}

func (r *registrationRunner) performRegistration(username, credsPath string) (*webauthn.Credential, error) {
	baseURL := "https://app.hourglass-app.com"
	if envURL := os.Getenv("HOURGLASS_URL"); envURL != "" {
		baseURL = envURL
	}

	auth, err := webauthn.NewAuthenticator(credsPath, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticator: %w", err)
	}

	return auth.Register(username)
}

func (r *registrationRunner) printSuccess(creds *webauthn.Credential, credsPath string) {
	var b strings.Builder

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "✅✅✅ Registration Successful! ✅✅✅")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "🔑 Credential registered:")
	fmt.Fprintf(&b, "   User: %s\n", creds.UserName)
	fmt.Fprintf(&b, "   ID: %s...%s\n", creds.ID[:8], creds.ID[len(creds.ID)-8:])
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "💾 Credentials saved to: %s\n", credsPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "🚀 Next steps:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "1. Copy credentials to your VPS:")
	fmt.Fprintf(&b, "   scp %s user@vps:~/.hourglass-rpa/\n", credsPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "2. Run the application with WebAuthn enabled:")
	fmt.Fprintln(&b, "   ./rpa -webauthn")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "3. Or set environment variable:")
	fmt.Fprintln(&b, "   export WEBAUTHN_ENABLED=true")
	fmt.Fprintln(&b, "   ./rpa")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "📋 The application will now automatically:")
	fmt.Fprintln(&b, "   • Renew tokens before they expire")
	fmt.Fprintln(&b, "   • Handle authentication without manual intervention")
	fmt.Fprintln(&b, "   • Store tokens securely (0600 permissions)")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "⚠️  Important:")
	fmt.Fprintln(&b, "   • Keep credentials file secure (it's already 0600)")
	fmt.Fprintln(&b, "   • Back up the credentials file - losing it requires re-registration")
	fmt.Fprintln(&b, "   • Each environment (local, VPS, etc.) needs its own registration")

	fmt.Print(b.String())
}
