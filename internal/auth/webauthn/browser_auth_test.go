package webauthn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBrowserAuth(t *testing.T) {
	t.Run("new browser auth", func(t *testing.T) {
		ba := NewBrowserAuth("https://example.com")
		assert.NotNil(t, ba)
		assert.Equal(t, "https://example.com", ba.baseURL)
		assert.True(t, ba.headless)
	})

	t.Run("with headless false", func(t *testing.T) {
		ba := NewBrowserAuth("https://example.com").WithHeadless(false)
		assert.NotNil(t, ba)
		assert.False(t, ba.headless)
	})

	t.Run("with headless true", func(t *testing.T) {
		ba := NewBrowserAuth("https://example.com").WithHeadless(true)
		assert.NotNil(t, ba)
		assert.True(t, ba.headless)
	})
}

func TestGetChromePath(t *testing.T) {
	t.Run("returns path", func(t *testing.T) {
		path := getChromePath()
		assert.NotEmpty(t, path)
	})
}
