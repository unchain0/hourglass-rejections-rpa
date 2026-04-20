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
	service.captureAnalysisError(errors.New("boom"), 123, "Field Ministry", time.Now(), "phase", nil)

	service = newManualCheckService(nil, &telemetry.Client{}, nil, nil, nil)
	service.captureAnalysisError(errors.New("boom"), 123, "Field Ministry", time.Now(), "phase", &domain.JobResult{Total: 2})
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
