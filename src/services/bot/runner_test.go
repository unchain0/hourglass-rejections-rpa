package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/i18n"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"
	"hourglass-rejections-rpa/src/services/notification"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	origNewPreferenceStore := newPreferenceStoreFromDatabaseURL
	newPreferenceStoreFromDatabaseURL = func(databaseURL string) (preferences.PreferenceStore, error) {
		store, err := preferences.NewStore(filepath.Join(os.TempDir(), fmt.Sprintf("hourglass-bot-test-%d.db", time.Now().UnixNano())))
		if err != nil {
			return nil, err
		}
		return store, nil
	}
	defer func() { newPreferenceStoreFromDatabaseURL = origNewPreferenceStore }()

	if err := i18n.Init(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestOpenPreferenceStoreFromDatabaseURLRejectsInvalidDSN(t *testing.T) {
	store, err := openPreferenceStoreFromDatabaseURL("://invalid")
	assert.Error(t, err)
	assert.Nil(t, store)
}

func TestEnsurePreferenceStoreReturnsFactoryError(t *testing.T) {
	original := newPreferenceStoreFromDatabaseURL
	t.Cleanup(func() { newPreferenceStoreFromDatabaseURL = original })
	newPreferenceStoreFromDatabaseURL = func(string) (preferences.PreferenceStore, error) {
		return nil, errors.New("open failed")
	}

	runner := &BotRunner{cfg: &config.Config{DatabaseURL: "configured"}}
	store, closeStore, err := runner.ensurePreferenceStore()
	assert.Error(t, err)
	assert.Nil(t, store)
	assert.NotNil(t, closeStore)
}

func TestNoOpPreferenceStoreClose(t *testing.T) {
	assert.NotPanics(t, noOpPreferenceStoreClose)
}

func TestSendRejectionsReturnsNilPreferenceStoreError(t *testing.T) {
	original := newPreferenceStoreFromDatabaseURL
	t.Cleanup(func() { newPreferenceStoreFromDatabaseURL = original })
	newPreferenceStoreFromDatabaseURL = func(string) (preferences.PreferenceStore, error) {
		return nil, nil
	}

	runner := &BotRunner{cfg: &config.Config{DatabaseURL: "configured"}}
	err := runner.SendRejections([]domain.Rejection{{Section: "Field Ministry"}})
	assert.EqualError(t, err, "preference store not configured")
}

func TestSendRejectionsReturnsPreferenceStoreFactoryError(t *testing.T) {
	original := newPreferenceStoreFromDatabaseURL
	t.Cleanup(func() { newPreferenceStoreFromDatabaseURL = original })
	expected := errors.New("factory failed")
	newPreferenceStoreFromDatabaseURL = func(string) (preferences.PreferenceStore, error) {
		return nil, expected
	}

	runner := &BotRunner{cfg: &config.Config{DatabaseURL: "configured"}}
	assert.ErrorIs(t, runner.SendRejections([]domain.Rejection{{Section: "Field Ministry"}}), expected)
}

type MockAnalyzer struct {
	AnalyzeSectionFunc func(section string) (*domain.JobResult, error)
}

func (m *MockAnalyzer) AnalyzeSection(section string) (*domain.JobResult, error) {
	if m.AnalyzeSectionFunc != nil {
		return m.AnalyzeSectionFunc(section)
	}
	return &domain.JobResult{}, nil
}

type MockNotifier struct {
	StartBotFunc                   func(ctx context.Context, prefManager *preferences.PreferenceManager) error
	StopBotFunc                    func() error
	SetCheckNowCallbackFunc        func(callback notifier.CheckNowCallback)
	SendNoRejectionsMessageFunc    func(chatID int64, message string) error
	SendRejectionsNotificationFunc func(chatID int64, rejections []domain.Rejection) error
}

func (m *MockNotifier) StartBot(ctx context.Context, prefManager *preferences.PreferenceManager) error {
	if m.StartBotFunc != nil {
		return m.StartBotFunc(ctx, prefManager)
	}
	return nil
}

func (m *MockNotifier) StopBot() error {
	if m.StopBotFunc != nil {
		return m.StopBotFunc()
	}
	return nil
}

func (m *MockNotifier) SetCheckNowCallback(callback notifier.CheckNowCallback) {
	if m.SetCheckNowCallbackFunc != nil {
		m.SetCheckNowCallbackFunc(callback)
	}
}

func (m *MockNotifier) SendNoRejectionsMessage(chatID int64, message string) error {
	if m.SendNoRejectionsMessageFunc != nil {
		return m.SendNoRejectionsMessageFunc(chatID, message)
	}
	return nil
}

func (m *MockNotifier) SendRejectionsNotification(chatID int64, rejections []domain.Rejection) error {
	if m.SendRejectionsNotificationFunc != nil {
		return m.SendRejectionsNotificationFunc(chatID, rejections)
	}
	return nil
}

type MockPreferenceStore struct {
	CloseFunc  func() error
	GetFunc    func(chatID int64) (*preferences.UserPreference, error)
	SaveFunc   func(pref *preferences.UserPreference) error
	DeleteFunc func(chatID int64) error
	ListFunc   func() ([]preferences.UserPreference, error)
}

func (m *MockPreferenceStore) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockPreferenceStore) Get(chatID int64) (*preferences.UserPreference, error) {
	if m.GetFunc != nil {
		return m.GetFunc(chatID)
	}
	return nil, nil
}

func (m *MockPreferenceStore) Save(pref *preferences.UserPreference) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(pref)
	}
	return nil
}

func (m *MockPreferenceStore) Delete(chatID int64) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(chatID)
	}
	return nil
}

func (m *MockPreferenceStore) List() ([]preferences.UserPreference, error) {
	if m.ListFunc != nil {
		return m.ListFunc()
	}
	return nil, nil
}

func TestNew(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil)
	if runner == nil {
		t.Fatal("expected runner to not be nil")
	}
	if runner.cfg != cfg {
		t.Errorf("expected cfg to be %v, got %v", cfg, runner.cfg)
	}
}

func TestWithMethods(t *testing.T) {
	runner := &BotRunner{}

	mockNotifier := &MockNotifier{}
	runner.WithNotifier(mockNotifier)
	if runner.notifier != mockNotifier {
		t.Errorf("expected notifier to be set")
	}

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)
	if runner.prefStore != mockStore {
		t.Errorf("expected prefStore to be set")
	}

	mockAnalyzer := &MockAnalyzer{}
	runner.WithAnalyzer(mockAnalyzer)
	if runner.analyzer != mockAnalyzer {
		t.Errorf("expected analyzer to be set")
	}
}

func TestBotRunnerSendRejectionsFansOutByEnabledPreferences(t *testing.T) {
	fieldMinistry := preferences.UserPreference{ChatID: 101, Enabled: true, Authorized: true}
	fieldMinistry.SetSections([]string{"Field Ministry"})
	publicWitnessing := preferences.UserPreference{ChatID: 202, Enabled: true, Authorized: true}
	publicWitnessing.SetSections([]string{"Public Witnessing"})
	disabled := preferences.UserPreference{ChatID: 303, Enabled: false}
	disabled.SetSections([]string{"Field Ministry", "Public Witnessing"})

	deliveries := make(map[int64][]domain.Rejection)
	runner := (&BotRunner{cfg: &config.Config{TelegramWhitelist: "101,202,303"}, telemetryClient: &telemetry.Client{}}).
		WithPreferenceStore(&MockPreferenceStore{ListFunc: func() ([]preferences.UserPreference, error) {
			return []preferences.UserPreference{fieldMinistry, publicWitnessing, disabled}, nil
		}}).
		WithNotifier(&MockNotifier{SendRejectionsNotificationFunc: func(chatID int64, rejections []domain.Rejection) error {
			deliveries[chatID] = append([]domain.Rejection(nil), rejections...)
			return nil
		}})

	err := runner.SendRejections([]domain.Rejection{
		{Section: "Field Ministry", Who: "A"},
		{Section: "Public Witnessing", Who: "B"},
	})

	assert.NoError(t, err)
	assert.Equal(t, []domain.Rejection{{Section: "Field Ministry", Who: "A"}}, deliveries[101])
	assert.Equal(t, []domain.Rejection{{Section: "Public Witnessing", Who: "B"}}, deliveries[202])
	assert.NotContains(t, deliveries, int64(303))
}

func TestBotRunnerSendRejectionsFansOutToEveryConfiguredWhitelistID(t *testing.T) {
	pref := preferences.UserPreference{ChatID: 202, Enabled: true}
	pref.SetSections([]string{"Field Ministry"})
	deliveries := make(map[int64]bool)

	runner := (&BotRunner{cfg: &config.Config{TelegramWhitelist: "101,202"}, telemetryClient: &telemetry.Client{}}).
		WithPreferenceStore(&MockPreferenceStore{ListFunc: func() ([]preferences.UserPreference, error) {
			return []preferences.UserPreference{pref}, nil
		}}).
		WithNotifier(&MockNotifier{SendRejectionsNotificationFunc: func(chatID int64, _ []domain.Rejection) error {
			deliveries[chatID] = true
			return nil
		}})

	require.NoError(t, runner.SendRejections([]domain.Rejection{{Section: "Field Ministry", Who: "A"}}))
	assert.True(t, deliveries[202])
}
func TestBotRunnerSendRejectionsBoundaries(t *testing.T) {
	rejection := domain.Rejection{Section: "Field Ministry", Who: "A"}

	t.Run("empty snapshot", func(t *testing.T) {
		assert.NoError(t, (&BotRunner{}).SendRejections(nil))
	})

	t.Run("missing preference store", func(t *testing.T) {
		err := (&BotRunner{}).SendRejections([]domain.Rejection{rejection})
		assert.EqualError(t, err, "preference store not configured")
	})

	t.Run("preference list failure", func(t *testing.T) {
		runner := (&BotRunner{}).WithPreferenceStore(&MockPreferenceStore{ListFunc: func() ([]preferences.UserPreference, error) {
			return nil, errors.New("list failed")
		}})
		err := runner.SendRejections([]domain.Rejection{rejection})
		assert.ErrorContains(t, err, "failed to list notification preferences: list failed")
	})

	t.Run("disabled unlisted and unmatched users", func(t *testing.T) {
		disabled := preferences.UserPreference{ChatID: 101, Enabled: false}
		disabled.SetSections([]string{"Field Ministry"})
		unlisted := preferences.UserPreference{ChatID: 202, Enabled: true}
		unlisted.SetSections([]string{"Field Ministry"})
		unmatched := preferences.UserPreference{ChatID: 101, Enabled: true}
		unmatched.SetSections([]string{"Public Witnessing"})
		runner := (&BotRunner{cfg: &config.Config{TelegramWhitelist: "101"}}).
			WithPreferenceStore(&MockPreferenceStore{ListFunc: func() ([]preferences.UserPreference, error) {
				return []preferences.UserPreference{disabled, unlisted, unmatched}, nil
			}})
		assert.NoError(t, runner.SendRejections([]domain.Rejection{rejection}))
	})

	t.Run("notifier initialization failure", func(t *testing.T) {
		t.Setenv("TELEGRAM_BOT_TOKEN", "")
		pref := preferences.UserPreference{ChatID: 101, Enabled: true}
		pref.SetSections([]string{"Field Ministry"})
		runner := (&BotRunner{cfg: &config.Config{TelegramWhitelist: "101"}}).
			WithPreferenceStore(&MockPreferenceStore{ListFunc: func() ([]preferences.UserPreference, error) {
				return []preferences.UserPreference{pref}, nil
			}})
		err := runner.SendRejections([]domain.Rejection{rejection})
		assert.ErrorContains(t, err, "failed to create telegram notifier")
	})

	t.Run("delivery failure", func(t *testing.T) {
		pref := preferences.UserPreference{ChatID: 101, Enabled: true}
		pref.SetSections([]string{"Field Ministry"})
		runner := (&BotRunner{cfg: &config.Config{}}).
			WithPreferenceStore(&MockPreferenceStore{ListFunc: func() ([]preferences.UserPreference, error) {
				return []preferences.UserPreference{pref}, nil
			}}).
			WithNotifier(&MockNotifier{SendRejectionsNotificationFunc: func(int64, []domain.Rejection) error {
				return errors.New("send failed")
			}})
		err := runner.SendRejections([]domain.Rejection{rejection})
		assert.ErrorContains(t, err, "failed to send scheduled notification: send failed")
	})
}

func TestRun_Success(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	mockNotifier := &MockNotifier{
		StartBotFunc: func(ctx context.Context, prefManager *preferences.PreferenceManager) error {
			return nil
		},
		StopBotFunc: func() error {
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRun_StartBotError(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	expectedErr := errors.New("start bot error")
	mockNotifier := &MockNotifier{
		StartBotFunc: func(ctx context.Context, prefManager *preferences.PreferenceManager) error {
			return expectedErr
		},
	}
	runner.WithNotifier(mockNotifier)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	err := runner.Run(context.Background())
	if err == nil || err.Error() != "failed to start bot: start bot error" {
		t.Errorf("expected start bot error, got %v", err)
	}
}

func TestRun_StopBotError(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	mockNotifier := &MockNotifier{
		StartBotFunc: func(ctx context.Context, prefManager *preferences.PreferenceManager) error {
			return nil
		},
		StopBotFunc: func() error {
			return errors.New("stop bot error")
		},
	}
	runner.WithNotifier(mockNotifier)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRun_NoNotifier_NoToken(t *testing.T) {
	os.Unsetenv("TELEGRAM_BOT_TOKEN")

	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN not configured") {
		t.Errorf("expected TELEGRAM_BOT_TOKEN error, got %v", err)
	}
}

func TestRun_NoPreferenceStore_Error(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL not configured") {
		t.Errorf("expected preference store error, got %v", err)
	}
}

func TestRun_NoPreferenceStore_Success(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "postgres://test:test@localhost:5432/hourglass"}
	runner := New(cfg, nil, nil)

	mockNotifier := &MockNotifier{
		StartBotFunc: func(ctx context.Context, prefManager *preferences.PreferenceManager) error {
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRun_I18nInitError(t *testing.T) {
	originalI18nInit := i18nInit
	i18nInit = func() error {
		return errors.New("i18n init error")
	}
	defer func() { i18nInit = originalI18nInit }()

	runner := New(&config.Config{}, &telemetry.Client{}, nil)

	err := runner.Run(context.Background())
	if err == nil || err.Error() != "failed to initialize i18n: i18n init error" {
		t.Errorf("expected i18n init error, got %v", err)
	}
}

func TestRun_CheckNowCallback(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	var callback notifier.CheckNowCallback
	var mu sync.Mutex
	mockNotifier := &MockNotifier{
		SetCheckNowCallbackFunc: func(cb notifier.CheckNowCallback) {
			mu.Lock()
			defer mu.Unlock()
			callback = cb
		},
		StartBotFunc: func(ctx context.Context, prefManager *preferences.PreferenceManager) error {
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return nil, errors.New("get error")
		},
	}
	runner.WithPreferenceStore(mockStore)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	cb := callback
	mu.Unlock()
	if cb == nil {
		t.Fatal("expected callback to be set")
	}

	err := cb(context.Background(), 123)
	if err == nil || err.Error() != "failed to get user preferences: get error" {
		t.Errorf("expected get error, got %v", err)
	}

	cancel()
	<-errCh
}

func TestRunOnceForUser_GetError(t *testing.T) {
	runner := &BotRunner{}
	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return nil, errors.New("get error")
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err == nil || err.Error() != "failed to get user preferences: get error" {
		t.Errorf("expected get error, got %v", err)
	}
}

func TestRunOnceForUser_NotFound(t *testing.T) {
	runner := &BotRunner{}
	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return nil, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err == nil || err.Error() != "user preferences not found" {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestRunOnceForUser_NotFound_WithSentryClient(t *testing.T) {
	runner := &BotRunner{telemetryClient: &telemetry.Client{}}
	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return nil, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err == nil || err.Error() != "user preferences not found" {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestRunOnceForUser_ContextCanceled(t *testing.T) {
	runner := &BotRunner{
		analyzer: &MockAnalyzer{
			AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
				t.Fatal("analyzer should not be called when context is already canceled")
				return nil, nil
			},
		},
	}

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runner.runOnceForUser(ctx, prefManager, 123)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context canceled error, got %v", err)
	}
}

func TestRunOnceForUser_NoSections(t *testing.T) {
	runner := &BotRunner{}

	called := false
	mockNotifier := &MockNotifier{
		SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
			called = true
			// Message should be in English (default language)
			if message != "No sections selected. You will not receive notifications." {
				t.Errorf("unexpected message: %s", message)
			}
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendNoRejectionsMessage to be called")
	}
}

func TestRunOnceForUser_AnalyzeError(t *testing.T) {
	runner := &BotRunner{}

	called := false
	mockNotifier := &MockNotifier{
		SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
			called = true
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockAnalyzer := &MockAnalyzer{
		AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
			return nil, errors.New("analyze error")
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendNoRejectionsMessage to be called")
	}
}

func TestRunOnceForUser_AnalyzeError_WithSentryClient(t *testing.T) {
	runner := &BotRunner{telemetryClient: &telemetry.Client{}}

	called := false
	mockNotifier := &MockNotifier{
		SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
			called = true
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockAnalyzer := &MockAnalyzer{
		AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
			return nil, errors.New("analyze error")
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendNoRejectionsMessage to be called")
	}
}

func TestRunOnceForUser_ResultError(t *testing.T) {
	runner := &BotRunner{}

	called := false
	mockNotifier := &MockNotifier{
		SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
			called = true
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockAnalyzer := &MockAnalyzer{
		AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
			return &domain.JobResult{Error: errors.New("result error")}, nil
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendNoRejectionsMessage to be called")
	}
}

func TestRunOnceForUser_NoRejections(t *testing.T) {
	runner := &BotRunner{}

	called := false
	mockNotifier := &MockNotifier{
		SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
			called = true
			// Message should be in English (default language)
			if message != "✅ No rejections found in configured sections." {
				t.Errorf("unexpected message: %s", message)
			}
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockAnalyzer := &MockAnalyzer{
		AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
			return &domain.JobResult{Rejections: []domain.Rejection{}}, nil
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendNoRejectionsMessage to be called")
	}
}

func TestRunOnceForUser_WithRejections(t *testing.T) {
	runner := &BotRunner{}

	called := false
	mockNotifier := &MockNotifier{
		SendRejectionsNotificationFunc: func(chatID int64, rejections []domain.Rejection) error {
			called = true
			if len(rejections) != 1 {
				t.Errorf("expected 1 rejection, got %d", len(rejections))
			}
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockAnalyzer := &MockAnalyzer{
		AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
			return &domain.JobResult{Rejections: []domain.Rejection{{Section: "Field Ministry"}}}, nil
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})

	mockStore := &MockPreferenceStore{
		GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	}
	prefManager := preferences.NewPreferenceManager(mockStore)

	err := runner.runOnceForUser(context.Background(), prefManager, 123)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendRejectionsNotification to be called")
	}
}

func TestSortRejectionsByDate(t *testing.T) {
	now := time.Date(2026, time.August, 8, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		rejections []domain.Rejection
		expected   []string
	}{
		{
			name: "past dates first and closest within each period",
			rejections: []domain.Rejection{
				{When: "2026-08-10"},
				{When: "2026-08-01"},
				{When: "2026-08-08"},
				{When: "2026-08-07"},
				{When: "2026-08-09"},
			},
			expected: []string{"2026-08-07", "2026-08-01", "2026-08-08", "2026-08-09", "2026-08-10"},
		},
		{
			name: "invalid dates stay last and stable",
			rejections: []domain.Rejection{
				{When: "unknown-1"},
				{When: "2026-08-09"},
				{When: "unknown-2"},
				{When: "2026-08-07"},
			},
			expected: []string{"2026-08-07", "2026-08-09", "unknown-1", "unknown-2"},
		},
		{
			name: "one invalid date sorts after valid date",
			rejections: []domain.Rejection{
				{When: "unknown"},
				{When: "2026-08-09"},
			},
			expected: []string{"2026-08-09", "unknown"},
		},
		{
			name: "valid date compares against invalid date",
			rejections: []domain.Rejection{
				{When: "2026-08-09"},
				{When: "unknown"},
			},
			expected: []string{"2026-08-09", "unknown"},
		},
		{
			name: "equal dates preserve source order",
			rejections: []domain.Rejection{
				{When: "2026-08-09", Who: "first"},
				{When: "2026-08-09", Who: "second"},
			},
			expected: []string{"first", "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortRejectionsByDate(tt.rejections, now)

			actual := make([]string, 0, len(tt.rejections))
			for _, rejection := range tt.rejections {
				if rejection.Who != "" {
					actual = append(actual, rejection.Who)
					continue
				}
				actual = append(actual, rejection.When)
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestRunOnceForUser_SortsRejectionsByDate(t *testing.T) {
	runner := &BotRunner{}

	mockNotifier := &MockNotifier{
		SendRejectionsNotificationFunc: func(_ int64, rejections []domain.Rejection) error {
			actual := make([]string, 0, len(rejections))
			for _, rejection := range rejections {
				actual = append(actual, rejection.When)
			}
			assert.Equal(t, []string{"2026-01-02", "2025-01-02", "2999-01-02", "2999-01-03"}, actual)
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)
	runner.WithAnalyzer(&MockAnalyzer{
		AnalyzeSectionFunc: func(string) (*domain.JobResult, error) {
			return &domain.JobResult{Rejections: []domain.Rejection{
				{When: "2999-01-03"},
				{When: "2025-01-02"},
				{When: "2999-01-02"},
				{When: "2026-01-02"},
			}}, nil
		},
	})

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})
	prefManager := preferences.NewPreferenceManager(&MockPreferenceStore{
		GetFunc: func(int64) (*preferences.UserPreference, error) {
			return pref, nil
		},
	})

	assert.NoError(t, runner.runOnceForUser(context.Background(), prefManager, 123))
}

func TestSendNoRejectionsMessage_NoNotifier_NoToken(t *testing.T) {
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	runner := &BotRunner{}

	err := runner.sendNoRejectionsMessage(123, "msg")
	if err == nil || err.Error() != "TELEGRAM_BOT_TOKEN not configured" {
		t.Errorf("expected TELEGRAM_BOT_TOKEN error, got %v", err)
	}
}

func TestSendNoRejectionsMessage_NoNotifier_WithToken(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")

	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return nil, errors.New("mock error")
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	runner := &BotRunner{}

	err := runner.sendNoRejectionsMessage(123, "msg")
	if err == nil || !strings.Contains(err.Error(), "failed to create telegram notifier") {
		t.Errorf("expected create telegram notifier error, got %v", err)
	}
}

func TestSendNoRejectionsMessage_NoNotifier_WithToken_AndSentry(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")

	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return nil, errors.New("mock error")
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	runner := &BotRunner{telemetryClient: &telemetry.Client{}}
	err := runner.sendNoRejectionsMessage(123, "msg")
	if err == nil || !strings.Contains(err.Error(), "failed to create telegram notifier") {
		t.Errorf("expected create telegram notifier error, got %v", err)
	}
}

func TestSendNoRejectionsMessage_Success(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")

	called := false
	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return &MockNotifier{
			SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
				called = true
				return nil
			},
		}, nil
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	runner := &BotRunner{}

	err := runner.sendNoRejectionsMessage(123, "msg")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendNoRejectionsMessage to be called")
	}
}

func TestSendRejectionsNotification_NoNotifier_NoToken(t *testing.T) {
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	runner := &BotRunner{}

	err := runner.sendRejectionsNotification(123, nil)
	if err == nil || err.Error() != "TELEGRAM_BOT_TOKEN not configured" {
		t.Errorf("expected TELEGRAM_BOT_TOKEN error, got %v", err)
	}
}

func TestSendRejectionsNotification_NoNotifier_WithToken(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")

	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return nil, errors.New("mock error")
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	runner := &BotRunner{}

	err := runner.sendRejectionsNotification(123, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to create telegram notifier") {
		t.Errorf("expected create telegram notifier error, got %v", err)
	}
}

func TestSendRejectionsNotification_Success(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")

	called := false
	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return &MockNotifier{
			SendRejectionsNotificationFunc: func(chatID int64, rejections []domain.Rejection) error {
				called = true
				return nil
			},
		}, nil
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	runner := &BotRunner{}

	err := runner.sendRejectionsNotification(123, nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Errorf("expected SendRejectionsNotification to be called")
	}
}

func TestGetWhitelist(t *testing.T) {
	_ = os.Setenv("TELEGRAM_WHITELIST", "123, 456, invalid, 789")
	defer func() { _ = os.Unsetenv("TELEGRAM_WHITELIST") }()

	runner := &BotRunner{}
	whitelist := runner.getWhitelist()

	if len(whitelist) != 3 {
		t.Errorf("expected 3 items, got %d", len(whitelist))
	}
	if whitelist[0] != 123 || whitelist[1] != 456 || whitelist[2] != 789 {
		t.Errorf("unexpected whitelist: %v", whitelist)
	}
}

func TestGetBotToken(t *testing.T) {
	t.Run("prefers config token", func(t *testing.T) {
		t.Setenv("TELEGRAM_BOT_TOKEN", "env-token")
		runner := &BotRunner{cfg: &config.Config{TelegramBotToken: "config-token"}}
		if token := runner.getBotToken(); token != "config-token" {
			t.Fatalf("expected config token, got %s", token)
		}
	})

	t.Run("falls back to env token", func(t *testing.T) {
		t.Setenv("TELEGRAM_BOT_TOKEN", "env-token")
		runner := &BotRunner{}
		if token := runner.getBotToken(); token != "env-token" {
			t.Fatalf("expected env token, got %s", token)
		}
	})
}

func TestManualCheckService_CaptureAnalysisError(t *testing.T) {
	service := newManualCheckService(nil, nil, nil, nil, nil)
	service.captureAnalysisError(errors.New("boom"), "Field Ministry", time.Now(), "phase", nil)

	service = newManualCheckService(nil, &telemetry.Client{}, nil, nil, nil)
	service.captureAnalysisError(errors.New("boom"), "Field Ministry", time.Now(), "phase", &domain.JobResult{Total: 2})
}

func TestManualCheckService_Run_NoSections(t *testing.T) {
	called := false
	pref := &preferences.UserPreference{}
	pref.SetSections([]string{})
	pm := preferences.NewPreferenceManager(&MockPreferenceStore{GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
		return pref, nil
	}})

	service := newManualCheckService(&MockAnalyzer{}, nil, pm, func(chatID int64, message string) error {
		called = true
		return nil
	}, nil)

	err := service.run(context.Background(), 123)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected no-rejections sender to be called")
	}
}

func TestManualCheckService_Run_WithRejections(t *testing.T) {
	called := false
	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Field Ministry"})
	pm := preferences.NewPreferenceManager(&MockPreferenceStore{GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
		return pref, nil
	}})

	service := newManualCheckService(&MockAnalyzer{AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
		return &domain.JobResult{Rejections: []domain.Rejection{{Section: section}}}, nil
	}}, nil, pm, func(chatID int64, message string) error {
		return nil
	}, func(chatID int64, rejections []domain.Rejection) error {
		called = true
		return nil
	})

	err := service.run(context.Background(), 123)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected rejections sender to be called")
	}
}

func TestManualCheckService_Run_PreferenceError(t *testing.T) {
	pm := preferences.NewPreferenceManager(&MockPreferenceStore{GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
		return nil, errors.New("get error")
	}})
	service := newManualCheckService(&MockAnalyzer{}, nil, pm, nil, nil)
	err := service.run(context.Background(), 123)
	if err == nil || !strings.Contains(err.Error(), "failed to get user preferences") {
		t.Fatalf("expected preference error, got %v", err)
	}
}

func TestManualCheckService_Run_PreferenceNotFound(t *testing.T) {
	pm := preferences.NewPreferenceManager(&MockPreferenceStore{GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
		return nil, nil
	}})
	service := newManualCheckService(&MockAnalyzer{}, nil, pm, nil, nil)
	err := service.run(context.Background(), 123)
	if err == nil || err.Error() != "user preferences not found" {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestManualCheckService_Run_PreferenceError_WithSentry(t *testing.T) {
	pm := preferences.NewPreferenceManager(&MockPreferenceStore{GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
		return nil, errors.New("get error")
	}})
	service := newManualCheckService(&MockAnalyzer{}, &telemetry.Client{}, pm, nil, nil)
	err := service.run(context.Background(), 123)
	if err == nil || !strings.Contains(err.Error(), "failed to get user preferences") {
		t.Fatalf("expected preference error, got %v", err)
	}
}

func TestManualCheckService_Run_PreferenceNotFound_WithSentry(t *testing.T) {
	pm := preferences.NewPreferenceManager(&MockPreferenceStore{GetFunc: func(chatID int64) (*preferences.UserPreference, error) {
		return nil, nil
	}})
	service := newManualCheckService(&MockAnalyzer{}, &telemetry.Client{}, pm, nil, nil)
	err := service.run(context.Background(), 123)
	if err == nil || err.Error() != "user preferences not found" {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestRun_NoNotifier_WithToken_Success(t *testing.T) {
	_ = os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	_ = os.Setenv("TELEGRAM_WHITELIST", "12345")
	defer func() { _ = os.Unsetenv("TELEGRAM_BOT_TOKEN") }()
	defer func() { _ = os.Unsetenv("TELEGRAM_WHITELIST") }()

	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return &MockNotifier{
			StartBotFunc: func(ctx context.Context, prefManager *preferences.PreferenceManager) error {
				return nil
			},
		}, nil
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestNewTelegramNotifier(t *testing.T) {
	// Call the original newTelegramNotifier to cover it
	_, err := newTelegramNotifier("", 0, nil)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestRun_NoNotifier_WithToken(t *testing.T) {
	_ = os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	_ = os.Setenv("TELEGRAM_WHITELIST", "12345,invalid,67890")
	defer func() { _ = os.Unsetenv("TELEGRAM_BOT_TOKEN") }()
	defer func() { _ = os.Unsetenv("TELEGRAM_WHITELIST") }()

	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return nil, errors.New("mock error")
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	cfg := &config.Config{}
	runner := New(cfg, nil, nil)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "failed to create telegram notifier") {
		t.Errorf("expected create telegram notifier error, got %v", err)
	}
}
