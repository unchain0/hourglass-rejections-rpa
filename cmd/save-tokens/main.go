package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"hourglass-rejections-rpa/internal/auth/webauthn"
)

func main() {
	fmt.Println("🌐 Autenticação Hourglass + Salvamento de Tokens")
	fmt.Println("⏱️  Você tem 5 minutos para completar a autenticação")
	fmt.Println("👁️  A janela do Chrome será visível")
	fmt.Println()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Erro ao obter diretório home:", err)
	}

	configDir := filepath.Join(homeDir, ".hourglass-rpa")
	os.MkdirAll(configDir, 0700)

	tokensPath := filepath.Join(configDir, "auth-tokens.json")

	fmt.Printf("💾 Tokens serão salvos em: %s\n", tokensPath)
	fmt.Println()

	tm, err := webauthn.NewTokenManager(
		filepath.Join(configDir, "webauthn-credentials.json"),
		"https://app.hourglass-app.com",
		webauthn.WithTokensPath(tokensPath),
		webauthn.WithOnTokenRenewed(func(tokens *webauthn.AuthTokens) {
			fmt.Println("🔄 Tokens renovados!")
		}),
	)
	if err != nil {
		log.Fatal("Erro ao criar TokenManager:", err)
	}

	browserAuth := webauthn.NewBrowserAuth("https://app.hourglass-app.com").WithHeadless(false)

	tokens, err := browserAuth.Authenticate()
	if err != nil {
		log.Fatal("❌ Autenticação falhou:", err)
	}

	err = tm.SaveTokens(tokens)
	if err != nil {
		log.Fatal("❌ Erro ao salvar tokens:", err)
	}

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
