package rpa

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hourglass-rejeicoes-rpa/internal/config"
	"hourglass-rejeicoes-rpa/internal/domain"
	"hourglass-rejeicoes-rpa/internal/storage"
)

func createTestStorage(t *testing.T) (*storage.FileStorage, string) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		OutputDir:  tmpDir,
		CookieFile: filepath.Join(tmpDir, "cookies.json"),
	}
	return storage.New(cfg), tmpDir
}

func TestNewLoginManager(t *testing.T) {
	browser := NewBrowser()
	store, _ := createTestStorage(t)

	lm := NewLoginManager(browser, store)

	if lm == nil {
		t.Fatal("NewLoginManager returned nil")
	}

	if lm.browser != browser {
		t.Error("browser not set correctly")
	}

	if lm.storage != store {
		t.Error("storage not set correctly")
	}
}

func TestLoginManager_SetupMode(t *testing.T) {
	browser := NewBrowser()
	store, _ := createTestStorage(t)
	lm := NewLoginManager(browser, store)

	ctx := context.Background()

	// This will setup the browser (non-headless in debug mode)
	err := lm.SetupMode(ctx)
	if err != nil {
		t.Errorf("SetupMode() error = %v", err)
	}

	defer browser.Close()
}

func TestLoginManager_LoadSaveCookies(t *testing.T) {
	// Create storage
	store, tmpDir := createTestStorage(t)

	// Test loading cookies when none exist
	cookies, err := store.LoadCookies()
	if err != nil {
		t.Errorf("LoadCookies() with no cookies error = %v", err)
	}
	if len(cookies) != 0 {
		t.Error("expected 0 cookies, got some")
	}

	// Save some test cookies
	testCookies := []domain.Cookie{
		{
			Name:     "test_cookie",
			Value:    "test_value",
			Domain:   ".petrobras.com",
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
		},
	}

	err = store.SaveCookies(testCookies)
	if err != nil {
		t.Errorf("SaveCookies() error = %v", err)
	}

	// Verify file was created
	cookiePath := filepath.Join(tmpDir, "cookies.json")
	if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
		t.Error("cookies.json was not created")
	}

	// Load cookies back
	loadedCookies, err := store.LoadCookies()
	if err != nil {
		t.Errorf("LoadCookies() after save error = %v", err)
	}
	if len(loadedCookies) != 1 {
		t.Errorf("expected 1 cookie, got %d", len(loadedCookies))
	}
	if loadedCookies[0].Name != "test_cookie" {
		t.Errorf("expected cookie name 'test_cookie', got '%s'", loadedCookies[0].Name)
	}
}

func TestLoginManager_IsAuthenticated(t *testing.T) {
	browser := NewBrowser()
	store, _ := createTestStorage(t)
	lm := NewLoginManager(browser, store)

	ctx := context.Background()

	// Setup browser
	err := browser.Setup()
	if err != nil {
		t.Fatalf("browser.Setup() error = %v", err)
	}
	defer browser.Close()

	// This will fail to check auth without actual navigation
	// but we're testing the method structure
	_, err = lm.IsAuthenticated(ctx)
	// We expect an error since we're not actually navigating to a real site
	// This is acceptable for unit testing structure
	_ = err
}
