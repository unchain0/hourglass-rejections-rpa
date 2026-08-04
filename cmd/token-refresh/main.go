// Package main provides a command-line tool for refreshing authentication tokens.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"

	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

const (
	defaultBaseURL          = "https://app.hourglass-app.com"
	defaultRefreshThreshold = 6 * time.Hour
)

type tokenManager interface {
	LoadTokens() (*webauthn.AuthTokens, error)
	EnsureValidTokens() (*webauthn.AuthTokens, error)
	PrimeTokens(tokens *webauthn.AuthTokens)
}

type tokenManagerFactory func(string, string, ...webauthn.TokenManagerOption) (tokenManager, error)

type tokenRefresher struct {
	userHomeDir         func() (string, error)
	getenv              func(string) string
	tokenManagerFactory tokenManagerFactory
	baseURL             string
}

func newTokenRefresher() *tokenRefresher {
	baseURL := os.Getenv("HOURGLASS_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &tokenRefresher{
		userHomeDir: os.UserHomeDir,
		getenv:      os.Getenv,
		tokenManagerFactory: func(credentialsPath, baseURL string, opts ...webauthn.TokenManagerOption) (tokenManager, error) {
			return webauthn.NewTokenManager(credentialsPath, baseURL, opts...)
		},
		baseURL: baseURL,
	}
}

var osExit = os.Exit
var newTokenRefresherFunc = newTokenRefresher

func main() {
	_ = godotenv.Load()
	tr := newTokenRefresherFunc()
	if err := tr.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		osExit(1)
	}
}

func (tr *tokenRefresher) Run() error {
	fmt.Println("🔄 Token Refresh - Renovação real de autenticação")
	fmt.Println()

	configDir, err := tr.configDir()
	if err != nil {
		return fmt.Errorf("erro ao obter diretório de configuração: %w", err)
	}

	credentialsPath := tr.credentialsPath(configDir)
	tokensPath := tr.tokensPath(configDir)
	profileDir := tr.getenv("CHROME_PROFILE_DIR")
	renewalThreshold := tr.renewalThreshold()

	fmt.Printf("🔐 Credenciais WebAuthn: %s\n", credentialsPath)
	fmt.Printf("💾 Tokens:               %s\n", tokensPath)
	if profileDir != "" {
		fmt.Printf("🌐 Perfil do Chrome:     %s\n", profileDir)
	}
	fmt.Printf("⏱️  Limite de renovação: %s\n", renewalThreshold)

	managerOptions := []webauthn.TokenManagerOption{
		webauthn.WithTokensPath(tokensPath),
		webauthn.WithRenewalThreshold(renewalThreshold),
	}
	if profileDir != "" {
		managerOptions = append(managerOptions, webauthn.WithBrowserProfileDir(profileDir))
	}

	tm, err := tr.tokenManagerFactory(credentialsPath, tr.baseURL, managerOptions...)
	if err != nil {
		return fmt.Errorf("erro ao criar gerenciador de tokens: %w", err)
	}

	currentTokens, err := tm.LoadTokens()
	if err != nil {
		return fmt.Errorf("erro ao carregar tokens atuais: %w", err)
	}

	if currentTokens == nil {
		currentTokens = tr.environmentSession()
		if currentTokens == nil {
			fmt.Println("📭 Nenhum token persistido ou sessão configurada encontrado.")
		} else {
			fmt.Println("🔑 Usando token e cookie do ambiente como sessão de bootstrap.")
			tm.PrimeTokens(currentTokens)
		}
	} else {
		fmt.Printf("📅 Tokens atuais válidos até: %s\n", currentTokens.ExpiresAt.Format("02/01/2006 15:04:05"))
		tm.PrimeTokens(currentTokens)
	}

	refreshedTokens, err := tm.EnsureValidTokens()
	if err != nil {
		fmt.Println()
		fmt.Println("💡 Para renovação automática funcionar, garanta um destes modos:")
		fmt.Println("   - CHROME_PROFILE_DIR apontando para um perfil autenticado do Chrome/Chromium")
		fmt.Println("   - ou make setup-auth + auth-tokens.json/webauthn-credentials.json")
		return fmt.Errorf("falha na renovação real dos tokens: %w", err)
	}

	fmt.Println()
	if tokensEqual(currentTokens, refreshedTokens) {
		fmt.Println("✅ Tokens já estavam válidos; nenhuma renovação foi necessária.")
	} else {
		fmt.Println("✅ Tokens renovados com sucesso!")
	}
	fmt.Printf("📅 Validated atual: %s\n", refreshedTokens.ExpiresAt.Format("02/01/2006 15:04:05"))

	return nil
}

func (tr *tokenRefresher) environmentSession() *webauthn.AuthTokens {
	hgLogin := tr.getenv("HOURGLASS_HGLOGIN_COOKIE")
	xsrfToken := tr.getenv("HOURGLASS_XSRF_TOKEN")
	if hgLogin == "" || xsrfToken == "" {
		return nil
	}

	return &webauthn.AuthTokens{
		HGLogin:   hgLogin,
		XSRFToken: xsrfToken,
		ExpiresAt: time.Now().Add(-time.Second),
	}
}

func (tr *tokenRefresher) configDir() (string, error) {
	homeDir, err := tr.userHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, ".hourglass-rpa"), nil
}

func (tr *tokenRefresher) credentialsPath(configDir string) string {
	if path := tr.getenv("WEBAUTHN_CREDENTIALS_PATH"); path != "" {
		return path
	}

	return filepath.Join(configDir, "webauthn-credentials.json")
}

func (tr *tokenRefresher) tokensPath(configDir string) string {
	if path := tr.getenv("WEBAUTHN_TOKENS_PATH"); path != "" {
		return path
	}

	if path := tr.getenv("TOKENS_PATH"); path != "" {
		return path
	}

	return filepath.Join(configDir, "auth-tokens.json")
}

func (tr *tokenRefresher) renewalThreshold() time.Duration {
	value := tr.getenv("REFRESH_INTERVAL")
	if value == "" {
		return defaultRefreshThreshold
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return defaultRefreshThreshold
	}

	return parsed
}

func tokensEqual(left, right *webauthn.AuthTokens) bool {
	if left == nil || right == nil {
		return left == right
	}

	return left.HGLogin == right.HGLogin &&
		left.XSRFToken == right.XSRFToken &&
		left.ExpiresAt.Equal(right.ExpiresAt)
}
