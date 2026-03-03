package webauthn

import (
	"errors"
	"testing"

	"github.com/chromedp/cdproto/network"
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

func TestExtractTokens(t *testing.T) {
	t.Run("extract tokens from cookies", func(t *testing.T) {
		cookies := []*network.Cookie{
			{Name: "hglogin", Value: "test-hglogin"},
			{Name: "X-Hourglass-XSRF-Token", Value: "test-xsrf"},
		}

		tokens, err := extractTokens(cookies)
		assert.NoError(t, err)
		assert.NotNil(t, tokens)
		assert.Equal(t, "test-hglogin", tokens.HGLogin)
		assert.Equal(t, "test-xsrf", tokens.XSRFToken)
		assert.False(t, tokens.IsExpired())
	})

	t.Run("missing cookies", func(t *testing.T) {
		cookies := []*network.Cookie{
			{Name: "other", Value: "value"},
		}

		tokens, err := extractTokens(cookies)
		assert.Error(t, err)
		assert.Nil(t, tokens)
	})
}

func TestHasAuthCookies(t *testing.T) {
	t.Run("has both cookies", func(t *testing.T) {
		cookies := []*network.Cookie{
			{Name: "hglogin", Value: "test"},
			{Name: "X-Hourglass-XSRF-Token", Value: "test"},
		}
		assert.True(t, hasAuthCookies(cookies))
	})

	t.Run("missing hglogin", func(t *testing.T) {
		cookies := []*network.Cookie{
			{Name: "X-Hourglass-XSRF-Token", Value: "test"},
		}
		assert.False(t, hasAuthCookies(cookies))
	})

	t.Run("missing xsrf", func(t *testing.T) {
		cookies := []*network.Cookie{
			{Name: "hglogin", Value: "test"},
		}
		assert.False(t, hasAuthCookies(cookies))
	})

	t.Run("no cookies", func(t *testing.T) {
		var cookies []*network.Cookie
		assert.False(t, hasAuthCookies(cookies))
	})
}

func TestIsTransientAuthError(t *testing.T) {
	t.Run("transient errors", func(t *testing.T) {
		transientErrors := []string{
			"net::ERR_CONNECTION_RESET",
			"target closed",
			"navigation timeout",
			"authentication timeout",
			"connection reset",
		}

		for _, errMsg := range transientErrors {
			err := errors.New(errMsg)
			assert.True(t, isTransientAuthError(err), "should be transient: %s", errMsg)
		}
	})

	t.Run("non-transient errors", func(t *testing.T) {
		nonTransientErrors := []string{
			"invalid credentials",
			"authentication failed",
			"not found",
		}

		for _, errMsg := range nonTransientErrors {
			err := errors.New(errMsg)
			assert.False(t, isTransientAuthError(err), "should not be transient: %s", errMsg)
		}
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, isTransientAuthError(nil))
	})
}
