package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"
	hourglass "hourglass-rejections-rpa/src/services/hourglass"
	"hourglass-rejections-rpa/src/services/scheduler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runnerFunc func(context.Context) error

func (f runnerFunc) Run(ctx context.Context) error {
	return f(ctx)
}

func (f runnerFunc) SendRejections([]domain.Rejection) error {
	return nil
}

func (f runnerFunc) SetNotifier(scheduler.RejectionNotifier) {}

type schedulerRunnerSpy struct {
	run      runnerFunc
	notifier scheduler.RejectionNotifier
}

func (s *schedulerRunnerSpy) Run(ctx context.Context) error {
	return s.run(ctx)
}

func (s *schedulerRunnerSpy) SetNotifier(notifier scheduler.RejectionNotifier) {
	s.notifier = notifier
}

type telegramRunnerSpy struct {
	run runnerFunc
}

func (t *telegramRunnerSpy) Run(ctx context.Context) error {
	return t.run(ctx)
}

func (t *telegramRunnerSpy) SendRejections([]domain.Rejection) error {
	return nil
}

func TestOpenPreferenceStore(t *testing.T) {
	t.Run("default sqlite", func(t *testing.T) {
		t.Setenv("DB_TYPE", "sqlite")
		t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "preferences.db"))
		store, err := openPreferenceStore(&config.Config{})
		require.NoError(t, err)
		require.NotNil(t, store)
		require.NoError(t, store.Close())
	})

	t.Run("invalid postgres DSN", func(t *testing.T) {
		store, err := openPreferenceStore(&config.Config{DatabaseURL: "://invalid"})
		require.Error(t, err)
		if store != nil {
			require.NoError(t, store.Close())
		}
	})
}

func TestRunReportsDependencySetupError(t *testing.T) {
	original := newPreferenceStore
	t.Cleanup(func() { newPreferenceStore = original })
	newPreferenceStore = func(*config.Config) (*preferences.Store, error) {
		return nil, errors.New("database unavailable")
	}
	t.Setenv("AUTO_REFRESH_TOKENS", "false")
	t.Setenv("TIMEOUT", "1s")

	err := run(t.Context(), runOptions{getenv: os.Getenv})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to setup dependencies")
}

func TestSetupTelemetryFallsBackAfterInitializationError(t *testing.T) {
	original := newTelemetryClient
	t.Cleanup(func() { newTelemetryClient = original })
	calls := 0
	newTelemetryClient = func(cfg telemetry.Config) (*telemetry.Client, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("initialization failed")
		}
		return original(cfg)
	}

	client := setupTelemetry(&config.Config{
		OTLPEndpoint:       "https://otel.example.com",
		OTLPMetricInterval: time.Second,
		LogLevel:           "debug",
	})

	require.NotNil(t, client)
	assert.False(t, client.IsEnabled())
}

func TestRegisterBootstrapWebAuthnCredential(t *testing.T) {
	t.Run("storage initialization error", func(t *testing.T) {
		err := registerBootstrapWebAuthnCredential("/proc/hourglass-rpa/credentials.json", "https://example.com", "xsrf", "hglogin")
		require.Error(t, err)
	})

	t.Run("registration success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v0.2/auth/webauthn/register/begin":
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"publicKey":{"rp":{"name":"Hourglass","id":"hourglass-app.com"},"user":{"id":"dXNlcg","name":"User","displayName":"User"},"challenge":"Y2hhbGxlbmdl","pubKeyCredParams":[{"type":"public-key","alg":-7}],"timeout":60000,"attestation":"none"}}`))
				require.NoError(t, err)
			case "/api/v0.2/auth/webauthn/register/finish":
				w.WriteHeader(http.StatusCreated)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
		err := registerBootstrapWebAuthnCredential(credentialsPath, server.URL, "xsrf", "hglogin")
		require.NoError(t, err)
		info, err := os.Stat(credentialsPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
}

func TestEnableWebAuthnTokenManagerReportsBootstrapError(t *testing.T) {
	originalBootstrap := bootstrapWebAuthnCredential
	originalEnable := enableWebAuthnClient
	t.Cleanup(func() {
		bootstrapWebAuthnCredential = originalBootstrap
		enableWebAuthnClient = originalEnable
	})
	bootstrapWebAuthnCredential = func(string, string, string, string) error {
		return errors.New("registration rejected")
	}
	enableWebAuthnClient = func(*hourglass.Client, string) error {
		t.Fatal("token manager must not be enabled after bootstrap failure")
		return nil
	}

	cfg := &config.Config{
		AutoRefreshTokens:       true,
		HourglassURL:            "https://example.com",
		HourglassXSRFToken:      "xsrf",
		HourglassHGLogin:        "hglogin",
		WebAuthnCredentialsPath: filepath.Join(t.TempDir(), "missing.json"),
	}
	assert.False(t, enableWebAuthnTokenManager(hourglass.NewClient(), cfg))
}

func TestRunFullModeCapturesBotError(t *testing.T) {
	originalBot := newBotRunnerFn
	originalScheduler := newSchedulerFn
	t.Cleanup(func() {
		newBotRunnerFn = originalBot
		newSchedulerFn = originalScheduler
	})

	botFinished := make(chan struct{})
	newBotRunnerFn = func(*config.Config, *telemetry.Client, *hourglass.APIAnalyzer, *preferences.Store) telegramRunner {
		return runnerFunc(func(context.Context) error {
			close(botFinished)
			return errors.New("bot failed")
		})
	}
	newSchedulerFn = func(*config.Config, *telemetry.Client, *hourglass.APIAnalyzer, *preferences.Store) schedulerRunner {
		return runnerFunc(func(context.Context) error {
			<-botFinished
			return nil
		})
	}

	client := hourglass.NewClient()
	err := runFullMode(t.Context(), &config.Config{}, &telemetry.Client{}, hourglass.NewAPIAnalyzer(client), newTestStore(t))
	require.NoError(t, err)
}

func TestRunFullModeWiresBotToScheduler(t *testing.T) {
	originalBot := newBotRunnerFn
	originalScheduler := newSchedulerFn
	t.Cleanup(func() {
		newBotRunnerFn = originalBot
		newSchedulerFn = originalScheduler
	})

	botRunner := &telegramRunnerSpy{run: func(context.Context) error { return nil }}
	schedSpy := &schedulerRunnerSpy{run: func(context.Context) error { return nil }}
	newBotRunnerFn = func(*config.Config, *telemetry.Client, *hourglass.APIAnalyzer, *preferences.Store) telegramRunner {
		return botRunner
	}
	newSchedulerFn = func(*config.Config, *telemetry.Client, *hourglass.APIAnalyzer, *preferences.Store) schedulerRunner {
		return schedSpy
	}

	client := hourglass.NewClient()
	err := runFullMode(t.Context(), &config.Config{}, &telemetry.Client{}, hourglass.NewAPIAnalyzer(client), newTestStore(t))
	require.NoError(t, err)
	assert.Same(t, botRunner, schedSpy.notifier)
}
