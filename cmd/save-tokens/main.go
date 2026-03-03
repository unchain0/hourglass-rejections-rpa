package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"hourglass-rejections-rpa/internal/auth/webauthn"
)

var osExit = os.Exit

type tokenSaver interface {
	SaveTokens(tokens *webauthn.AuthTokens) error
}

type browserAuthenticator interface {
	Authenticate() (*webauthn.AuthTokens, error)
	WithHeadless(headless bool) browserAuthenticator
}

type tokenSaverImpl struct {
	tokenManagerFactory func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error)
	browserAuthFactory  func(baseURL string) browserAuthenticator
	userHomeDir         func() (string, error)
	mkdirAll            func(path string, perm os.FileMode) error
}

func newTokenSaver() *tokenSaverImpl {
	return &tokenSaverImpl{
		tokenManagerFactory: func(credsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenSaver, error) {
			return webauthn.NewTokenManager(credsPath, baseURL, opts...)
		},
		browserAuthFactory: func(baseURL string) browserAuthenticator {
			return &browserAuthAdapter{webauthn.NewBrowserAuth(baseURL)}
		},
		userHomeDir: os.UserHomeDir,
		mkdirAll:    os.MkdirAll,
	}
}

type browserAuthAdapter struct {
	*webauthn.BrowserAuth
}

func (a *browserAuthAdapter) Authenticate() (*webauthn.AuthTokens, error) {
	return a.BrowserAuth.Authenticate()
}

func (a *browserAuthAdapter) WithHeadless(headless bool) browserAuthenticator {
	return &browserAuthAdapter{a.BrowserAuth.WithHeadless(headless)}
}

func (ts *tokenSaverImpl) run() error {
	fmt.Println("🌐 Autenticação Hourglass + Salvamento de Tokens")
	fmt.Println("⏱️  Você tem 5 minutos para completar a autenticação")
	fmt.Println("👁️  A janela do Chrome será visível")
	fmt.Println()

	homeDir, err := ts.userHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".hourglass-rpa")
	if err := ts.mkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	tokensPath := filepath.Join(configDir, "auth-tokens.json")

	fmt.Printf("💾 Tokens serão salvos em: %s\n", tokensPath)
	fmt.Println()

	tm, err := ts.tokenManagerFactory(
		filepath.Join(configDir, "webauthn-credentials.json"),
		"https://app.hourglass-app.com",
		webauthn.WithTokensPath(tokensPath),
		webauthn.WithOnTokenRenewed(func(tokens *webauthn.AuthTokens) {
			fmt.Println("🔄 Tokens renovados!")
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create TokenManager: %w", err)
	}

	browserAuth := ts.browserAuthFactory("https://app.hourglass-app.com").WithHeadless(false)

	tokens, err := browserAuth.Authenticate()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	err = tm.SaveTokens(tokens)
	if err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	return nil
}

func printSuccess(tokensPath string, tokens *webauthn.AuthTokens) {
	fmt.Println()
	fmt.Println("✅✅✅ SUCESSO! ✅✅✅")
	fmt.Println()
	fmt.Println("🔑 Tokens extraídos e salvos:")
	fmt.Printf("   HGLogin:  %s...%s\n", tokens.HGLogin[:4], tokens.HGLogin[len(tokens.HGLogin)-4:])
	fmt.Printf("   XSRF:     %s...%s\n", tokens.XSRFToken[:4], tokens.XSRFToken[len(tokens.XSRFToken)-4:])
	fmt.Printf("   Expira:   %s\n", tokens.ExpiresAt.Format("02/01/2006 15:04:05"))
	fmt.Println()
	fmt.Printf("💾 Arquivo: %s\n", tokensPath)
	fmt.Println()
	fmt.Println("🚀 Agora você pode copiar esse arquivo para a VPS:")
	fmt.Printf("   scp %s user@vps:~/.hourglass-rpa/\n", tokensPath)
	fmt.Println()
	fmt.Println("✅ E o sistema funcionará automaticamente na VPS!")
}

func main() {
	ts := newTokenSaver()
	if err := ts.run(); err != nil {
		log.Fatal(err)
	}

	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".hourglass-rpa")
	tokensPath := filepath.Join(configDir, "auth-tokens.json")

	tm, err := webauthn.NewTokenManager(
		filepath.Join(configDir, "webauthn-credentials.json"),
		"https://app.hourglass-app.com",
		webauthn.WithTokensPath(tokensPath),
	)
	if err != nil {
		log.Fatal("Failed to load tokens for display:", err)
	}

	tokens, err := tm.LoadTokens()
	if err != nil {
		log.Fatal("Failed to load saved tokens:", err)
	}

	printSuccess(tokensPath, tokens)
}
