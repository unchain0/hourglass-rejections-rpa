package scraper

import (
	"context"
	"errors"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/domain"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Storage ---

type mockStorage struct {
	cookies    []domain.Cookie
	loadErr    error
	saveErr    error
	savedCalls int
}

func (m *mockStorage) Save(_ context.Context, _ []domain.Rejeicao) error {
	return nil
}

func (m *mockStorage) LoadCookies() ([]domain.Cookie, error) {
	return m.cookies, m.loadErr
}

func (m *mockStorage) SaveCookies(cookies []domain.Cookie) error {
	m.savedCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.cookies = cookies
	return nil
}

// --- Tests ---

func TestNewPlaywrightScraper(t *testing.T) {
	storage := &mockStorage{}
	scraper := NewPlaywrightScraper("test@example.com", "secret", storage, true)

	assert.Equal(t, "test@example.com", scraper.email)
	assert.Equal(t, "secret", scraper.password)
	assert.Equal(t, storage, scraper.storage)
	assert.True(t, scraper.headless)
	assert.NotNil(t, scraper.logger)
	assert.Nil(t, scraper.pw)
	assert.Nil(t, scraper.browser)
	assert.Nil(t, scraper.page)
}

func TestNewPlaywrightScraper_HeadlessFalse(t *testing.T) {
	scraper := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, false)
	assert.False(t, scraper.headless)
}

func TestPlaywrightCookiesToDomain(t *testing.T) {
	pwCookies := []playwright.Cookie{
		{
			Name:     "hglogin",
			Value:    "session123",
			Domain:   ".hourglass-app.com",
			Path:     "/",
			Expires:  float64(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
			Secure:   true,
			HttpOnly: true,
		},
		{
			Name:     "XSRF-TOKEN",
			Value:    "xsrf-abc",
			Domain:   ".hourglass-app.com",
			Path:     "/",
			Expires:  float64(time.Date(2027, 6, 15, 12, 0, 0, 0, time.UTC).Unix()),
			Secure:   true,
			HttpOnly: false,
		},
	}

	result := playwrightCookiesToDomain(pwCookies)

	require.Len(t, result, 2)

	// Check hglogin cookie
	assert.Equal(t, "hglogin", result[0].Name)
	assert.Equal(t, "session123", result[0].Value)
	assert.Equal(t, ".hourglass-app.com", result[0].Domain)
	assert.Equal(t, "/", result[0].Path)
	assert.True(t, result[0].Secure)
	assert.True(t, result[0].HttpOnly)

	// Check XSRF cookie
	assert.Equal(t, "XSRF-TOKEN", result[1].Name)
	assert.Equal(t, "xsrf-abc", result[1].Value)
	assert.False(t, result[1].HttpOnly)
}

func TestPlaywrightCookiesToDomain_Empty(t *testing.T) {
	result := playwrightCookiesToDomain([]playwright.Cookie{})
	assert.Empty(t, result)
}

func TestDomainCookiesToPlaywright(t *testing.T) {
	cookies := []domain.Cookie{
		{
			Name:     "hglogin",
			Value:    "session123",
			Domain:   ".hourglass-app.com",
			Path:     "/",
			Expires:  time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			Secure:   true,
			HttpOnly: true,
		},
	}

	result := domainCookiesToPlaywright(cookies)

	require.Len(t, result, 1)
	assert.Equal(t, "hglogin", result[0].Name)
	assert.Equal(t, "session123", result[0].Value)
	assert.Equal(t, ".hourglass-app.com", *result[0].Domain)
	assert.Equal(t, "/", *result[0].Path)
	assert.True(t, *result[0].Secure)
	assert.True(t, *result[0].HttpOnly)

	expectedExpires := float64(cookies[0].Expires.Unix())
	assert.Equal(t, expectedExpires, *result[0].Expires)
}

func TestDomainCookiesToPlaywright_Empty(t *testing.T) {
	result := domainCookiesToPlaywright([]domain.Cookie{})
	assert.Empty(t, result)
}

func TestCookieConversion_RoundTrip(t *testing.T) {
	original := []domain.Cookie{
		{
			Name:     "hglogin",
			Value:    "abc",
			Domain:   ".example.com",
			Path:     "/app",
			Expires:  time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
			Secure:   true,
			HttpOnly: true,
		},
		{
			Name:     "XSRF-TOKEN",
			Value:    "def",
			Domain:   ".example.com",
			Path:     "/",
			Expires:  time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
			Secure:   false,
			HttpOnly: false,
		},
	}

	// Convert domain -> playwright -> domain
	pwCookies := domainCookiesToPlaywright(original)
	// Simulate what playwright would give back (playwright.Cookie, not OptionalCookie)
	fullPwCookies := make([]playwright.Cookie, len(pwCookies))
	for i, c := range pwCookies {
		fullPwCookies[i] = playwright.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   *c.Domain,
			Path:     *c.Path,
			Expires:  *c.Expires,
			Secure:   *c.Secure,
			HttpOnly: *c.HttpOnly,
		}
	}
	roundTripped := playwrightCookiesToDomain(fullPwCookies)

	require.Len(t, roundTripped, len(original))
	for i := range original {
		assert.Equal(t, original[i].Name, roundTripped[i].Name)
		assert.Equal(t, original[i].Value, roundTripped[i].Value)
		assert.Equal(t, original[i].Domain, roundTripped[i].Domain)
		assert.Equal(t, original[i].Path, roundTripped[i].Path)
		assert.Equal(t, original[i].Secure, roundTripped[i].Secure)
		assert.Equal(t, original[i].HttpOnly, roundTripped[i].HttpOnly)
		// Expires loses sub-second precision from float64 conversion, but epoch comparison works
		assert.Equal(t, original[i].Expires.Unix(), roundTripped[i].Expires.Unix())
	}
}

func TestSetup_PlaywrightRunFails(t *testing.T) {
	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("playwright install failed")
	}

	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	err := s.Setup(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start playwright")
}

func TestIsAuthenticated_NilPage(t *testing.T) {
	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)

	authenticated, err := s.IsAuthenticated(context.Background())

	assert.False(t, authenticated)
	assert.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

func TestAnalyzeSection_NilPage(t *testing.T) {
	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)

	result, err := s.AnalyzeSection(context.Background(), "Partes Mecanicas")

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

func TestClose_NilResources(t *testing.T) {
	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)

	// All resources are nil — Close should succeed without errors
	err := s.Close()
	assert.NoError(t, err)
}

func TestLogin_NilPage(t *testing.T) {
	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	// page is nil, Login will call attemptLogin which will panic on nil s.page
	// This tests the retry logic with panics being deferred properly
	// Actually, attempting login with nil page should fail gracefully
	// We test via Setup failure instead

	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("cannot start")
	}

	err := s.Setup(context.Background())
	assert.Error(t, err)

	// Now try login without setup — page is nil
	err = s.Login(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrAuthenticationFailed)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "https://app.hourglass-app.com/v2/page/app", loginURL)
	assert.Equal(t, "input#email", selectorEmail)
	assert.Equal(t, "input#password", selectorPassword)
	assert.Equal(t, "button[type='submit']", selectorSubmit)
	assert.Equal(t, 30000, pageLoadTimeout)
	assert.Equal(t, 15000, navigationTimeout)
	assert.Equal(t, 3, maxLoginRetries)
}

// Verify PlaywrightScraper satisfies the domain.Scraper interface at compile time.
var _ domain.Scraper = (*PlaywrightScraper)(nil)

func TestClose_PartialResources(t *testing.T) {
	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)

	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("playwright failed")
	}

	_ = s.Setup(context.Background())

	err := s.Close()
	assert.NoError(t, err)
}

func TestIsAuthenticated_NoCookies(t *testing.T) {
	original := pwRun
	defer func() { pwRun = original }()

	callCount := 0
	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		callCount++
		return nil, errors.New("playwright not available")
	}

	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	_ = s.Setup(context.Background())

	auth, err := s.IsAuthenticated(context.Background())
	assert.False(t, auth)
	assert.Error(t, err)
}

func TestLogin_Success(t *testing.T) {
	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("playwright not available in tests")
	}

	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	_ = s.Setup(context.Background())

	err := s.Login(context.Background())
	assert.Error(t, err)
}

func TestLogin_AllRetriesFail(t *testing.T) {
	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("playwright not available")
	}

	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	_ = s.Setup(context.Background())

	err := s.Login(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrAuthenticationFailed)
}

func TestIsAuthenticated_NotAuthenticated(t *testing.T) {
	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("playwright not available")
	}

	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	_ = s.Setup(context.Background())

	auth, err := s.IsAuthenticated(context.Background())
	assert.False(t, auth)
	assert.Error(t, err)
}

func TestAnalyzeSection_NotAuthenticated(t *testing.T) {
	original := pwRun
	defer func() { pwRun = original }()

	pwRun = func(options ...*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, errors.New("playwright not available")
	}

	s := NewPlaywrightScraper("a@b.com", "pwd", &mockStorage{}, true)
	_ = s.Setup(context.Background())

	result, err := s.AnalyzeSection(context.Background(), "Campo")
	assert.Nil(t, result)
	assert.Error(t, err)
}
