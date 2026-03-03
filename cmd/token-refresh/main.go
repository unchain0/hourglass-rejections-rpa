package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"hourglass-rejections-rpa/internal/auth/webauthn"
)

const (
	baseURL = "https://app.hourglass-app.com"
)

func main() {
	fmt.Println("🔄 Token Refresh - Tentando renovar tokens automaticamente")
	fmt.Println()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("❌ Erro ao obter diretório home: %v\n", err)
		os.Exit(1)
	}

	tokensPath := filepath.Join(homeDir, ".hourglass-rpa", "auth-tokens.json")

	tokens, err := loadTokens(tokensPath)
	if err != nil {
		fmt.Printf("❌ Erro ao carregar tokens: %v\n", err)
		fmt.Println("💡 Execute: make save-tokens")
		os.Exit(1)
	}

	fmt.Printf("📅 Tokens atuais válidos até: %s\n", tokens.ExpiresAt.Format("02/01/2006 15:04:05"))

	newTokens, err := tryRefresh(tokens)
	if err != nil {
		fmt.Printf("\n❌ Refresh automático falhou: %v\n", err)
		fmt.Println()
		fmt.Println("💡 Isso é normal quando:")
		fmt.Println("   - Tokens já expiraram completamente")
		fmt.Println("   - A sessão foi invalidada no servidor")
		fmt.Println("   - É necessário re-autenticar com WebAuthn")
		fmt.Println()
		fmt.Println("📝 Próximo passo:")
		fmt.Println("   make save-tokens")
		fmt.Println("   # Autentique manualmente no navegador")
		os.Exit(1)
	}

	err = saveTokens(tokensPath, newTokens)
	if err != nil {
		fmt.Printf("❌ Erro ao salvar tokens: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✅ Tokens renovados com sucesso!")
	fmt.Printf("📅 Nova validade: %s\n", newTokens.ExpiresAt.Format("02/01/2006 15:04:05"))
	fmt.Println()
	fmt.Println("🚀 Você pode copiar para a VPS:")
	fmt.Printf("   make copy-to-vps VPS=seu-usuario@ sua-vps.com\n")
}

func loadTokens(path string) (*webauthn.AuthTokens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("arquivo de tokens não encontrado")
	}

	var tokens webauthn.AuthTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("erro ao ler tokens: %w", err)
	}

	return &tokens, nil
}

func tryRefresh(tokens *webauthn.AuthTokens) (*webauthn.AuthTokens, error) {
	fmt.Println("🌐 Tentando refresh na API do Hourglass...")

	// Criar cliente HTTP
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", baseURL+"/api/v0.2/fsreport/users", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Cookie", fmt.Sprintf("hglogin=%s", tokens.HGLogin))
	req.Header.Add("X-Hourglass-XSRF-Token", tokens.XSRFToken)
	req.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	// Ler resposta
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API retornou status %d: %s", resp.StatusCode, string(body))
	}

	newTokens := &webauthn.AuthTokens{
		HGLogin:   tokens.HGLogin,
		XSRFToken: tokens.XSRFToken,
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	for _, cookie := range resp.Cookies() {
		switch cookie.Name {
		case "hglogin":
			if cookie.Value != "" {
				newTokens.HGLogin = cookie.Value
				fmt.Println("   📝 Novo hglogin recebido")
			}
		case "X-Hourglass-XSRF-Token":
			if cookie.Value != "" {
				newTokens.XSRFToken = cookie.Value
				fmt.Println("   📝 Novo XSRF token recebido")
			}
		}
	}

	return newTokens, nil
}

func saveTokens(path string, tokens *webauthn.AuthTokens) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
