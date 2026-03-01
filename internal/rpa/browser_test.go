package rpa

import (
	"context"
	"os"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
)

func TestBrowserSetup(t *testing.T) {
	b := NewBrowser()
	defer b.Close()

	err := b.Setup()
	assert.NoError(t, err)
	assert.NotNil(t, b.Context())

	// Verify if we can run a simple command
	var userAgent string
	err = chromedp.Run(b.Context(),
		chromedp.Evaluate(`navigator.userAgent`, &userAgent),
	)
	assert.NoError(t, err)
	assert.Contains(t, userAgent, "Mozilla/5.0")
}

func TestBrowserDebugMode(t *testing.T) {
	os.Setenv("DEBUG", "true")
	defer os.Unsetenv("DEBUG")

	b := NewBrowser()
	defer b.Close()

	err := b.Setup()
	assert.NoError(t, err)
	assert.NotNil(t, b.Context())
}
