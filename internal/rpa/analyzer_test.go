package rpa

import (
	"testing"

	"hourglass-rejeicoes-rpa/internal/config"
	"hourglass-rejeicoes-rpa/internal/storage"
)

func createTestAnalyzer(t *testing.T) (*Analyzer, *Browser, *LoginManager) {
	browser := NewBrowser()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		OutputDir:  tmpDir,
		CookieFile: tmpDir + "/cookies.json",
	}
	store := storage.New(cfg)
	lm := NewLoginManager(browser, store)
	analyzer := NewAnalyzer(browser, lm)
	return analyzer, browser, lm
}

func TestNewAnalyzer(t *testing.T) {
	analyzer, _, _ := createTestAnalyzer(t)

	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}

	if analyzer.browser == nil {
		t.Error("browser not set")
	}

	if analyzer.loginManager == nil {
		t.Error("loginManager not set")
	}
}
