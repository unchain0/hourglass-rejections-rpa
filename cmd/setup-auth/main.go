// Package setup-auth provides interactive authentication setup for Hourglass RPA.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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

var osExit = os.Exit

type setupOptions struct {
	getenv        func(string) string
	osUserHomeDir func() (string, error)
}

func main() {
	opts := setupOptions{
		getenv:        os.Getenv,
		osUserHomeDir: os.UserHomeDir,
	}

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Setup failed: %v\n", err)
		osExit(1)
	}
}

func run(opts setupOptions) error {
	fmt.Println("🔐 Hourglass Rejections RPA - Authentication Setup")
	fmt.Println("============================================")
	fmt.Println()

	homeDir, err := opts.osUserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, defaultConfigDir)
	tokensPath := filepath.Join(configDir, defaultTokensFile)

	fmt.Println("📍 Configuration Directory:", configDir)
	fmt.Println("📄 Tokens File:        ", tokensPath)
	fmt.Println()

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	existingTokens, err := checkExistingTokens(tokensPath)
	if err != nil {
		return fmt.Errorf("failed to check existing tokens: %w", err)
	}

	if existingTokens != nil {
		if !existingTokens.IsExpired() {
			fmt.Println("✅ Valid tokens found!")
			fmt.Printf("   ⏰ Expires: %s\n", existingTokens.ExpiresAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("   ⏳ Time remaining: %s\n", time.Until(existingTokens.ExpiresAt).Round(time.Minute))

			fmt.Print("\n🔄 Re-authenticate anyway? (yes/no): ")
			var confirm string
			_, _ = fmt.Scanln(&confirm)
			if strings.ToLower(confirm) != "yes" {
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

	browserAuth := webauthn.NewBrowserAuth(defaultBaseURL).WithHeadless(false)

	tokens, err := browserAuth.Authenticate()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("\n✅ Authentication successful!")
	fmt.Printf("   🔑 HGLogin Token:  %s...\n", truncate(tokens.HGLogin, 30))
	fmt.Printf("   🔒 XSRF Token:     %s...\n", truncate(tokens.XSRFToken, 30))
	fmt.Printf("   ⏰ Expires At:      %s\n", tokens.ExpiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   ⏳ Valid for:       %s\n", time.Until(tokens.ExpiresAt).Round(time.Minute))
	fmt.Println()

	if err := saveTokens(tokensPath, tokens); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	fmt.Println("💾 Tokens saved successfully!")
	fmt.Printf("   📁 Location: %s\n", tokensPath)
	fmt.Println()

	return askVPSUpload(tokensPath)
}

func checkExistingTokens(path string) (*webauthn.AuthTokens, error) {
	data, err := os.ReadFile(path)
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
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write tokens file: %w", err)
	}

	return nil
}

func askVPSUpload(tokensPath string) error {
	fmt.Println("📦 VPS Deployment")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("You can copy the authentication tokens to your VPS for remote deployment.")
	fmt.Println()

	fmt.Print("📡 Transfer tokens to VPS via SCP? (yes/no): ")
	var confirm string
	_, _ = fmt.Scanln(&confirm)

	if strings.ToLower(confirm) != "yes" {
		fmt.Println("\n✅ Setup complete!")
		return nil
	}

	fmt.Println()
	fmt.Print("🖥️  VPS host (user@host): ")
	var vpsHost string
	_, _ = fmt.Scanln(&vpsHost)

	if vpsHost == "" {
		fmt.Println("❌ VPS host cannot be empty")
		return nil
	}

	fmt.Print("📂 VPS target path (default: ~/.hourglass-rpa/auth-tokens.json): ")
	var vpsPath string
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		vpsPath = scanner.Text()
	}
	if vpsPath == "" {
		vpsPath = "~/.hourglass-rpa/auth-tokens.json"
	}

	fmt.Println()
	fmt.Println("📤 Transferring tokens to VPS...")

	cmd := exec.Command("scp", "-p", tokensPath, fmt.Sprintf("%s:%s", vpsHost, vpsPath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to transfer tokens: %w", err)
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
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
