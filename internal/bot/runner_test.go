package bot

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/notifier"
	"hourglass-rejections-rpa/internal/preferences"
)

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
	SendRejectionsNotificationFunc func(chatID int64, rejections []domain.Rejeicao) error
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

func (m *MockNotifier) SendRejectionsNotification(chatID int64, rejections []domain.Rejeicao) error {
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
	runner := New(cfg, nil, nil, nil)
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
	runner := New(cfg, nil, nil, nil)

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
	runner := New(cfg, nil, nil, nil)

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
	runner := New(cfg, nil, nil, nil)

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
	runner := New(cfg, nil, nil, nil)

	mockStore := &MockPreferenceStore{}
	runner.WithPreferenceStore(mockStore)

	err := runner.Run(context.Background())
	if err == nil || err.Error() != "TELEGRAM_BOT_TOKEN not configured" {
		t.Errorf("expected TELEGRAM_BOT_TOKEN error, got %v", err)
	}
}

func TestRun_NoPreferenceStore_Error(t *testing.T) {
	cfg := &config.Config{SQLiteDBPath: "/invalid/path/that/does/not/exist/db.sqlite"}
	runner := New(cfg, nil, nil, nil)

	err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to initialize preference store") {
		t.Errorf("expected preference store error, got %v", err)
	}
}

func TestRun_NoPreferenceStore_Success(t *testing.T) {
	cfg := &config.Config{SQLiteDBPath: "file::memory:?cache=shared"}
	runner := New(cfg, nil, nil, nil)

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

func TestRun_CheckNowCallback(t *testing.T) {
	cfg := &config.Config{}
	runner := New(cfg, nil, nil, nil)

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
	if err == nil || err.Error() != "user preferences not found for chat ID: 123" {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestRunOnceForUser_NoSections(t *testing.T) {
	runner := &BotRunner{}

	called := false
	mockNotifier := &MockNotifier{
		SendNoRejectionsMessageFunc: func(chatID int64, message string) error {
			called = true
			if message != "Você não tem nenhuma seção configurada para monitoramento." {
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
	pref.SetSections([]string{"Campo"})

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
	pref.SetSections([]string{"Campo"})

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
			if message != "✅ Nenhuma rejeição encontrada nas seções configuradas." {
				t.Errorf("unexpected message: %s", message)
			}
			return nil
		},
	}
	runner.WithNotifier(mockNotifier)

	mockAnalyzer := &MockAnalyzer{
		AnalyzeSectionFunc: func(section string) (*domain.JobResult, error) {
			return &domain.JobResult{Rejeicoes: []domain.Rejeicao{}}, nil
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Campo"})

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
		SendRejectionsNotificationFunc: func(chatID int64, rejections []domain.Rejeicao) error {
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
			return &domain.JobResult{Rejeicoes: []domain.Rejeicao{{Secao: "Campo"}}}, nil
		},
	}
	runner.WithAnalyzer(mockAnalyzer)

	pref := &preferences.UserPreference{}
	pref.SetSections([]string{"Campo"})

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
			SendRejectionsNotificationFunc: func(chatID int64, rejections []domain.Rejeicao) error {
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
	os.Setenv("TELEGRAM_CHAT_ID", "123, 456, invalid, 789")
	defer os.Unsetenv("TELEGRAM_CHAT_ID")

	runner := &BotRunner{}
	whitelist := runner.getWhitelist()

	if len(whitelist) != 3 {
		t.Errorf("expected 3 items, got %d", len(whitelist))
	}
	if whitelist[0] != 123 || whitelist[1] != 456 || whitelist[2] != 789 {
		t.Errorf("unexpected whitelist: %v", whitelist)
	}
}

func TestRun_NoNotifier_WithToken_Success(t *testing.T) {
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	os.Setenv("TELEGRAM_CHAT_ID", "12345")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("TELEGRAM_CHAT_ID")

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
	runner := New(cfg, nil, nil, nil)

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
	os.Setenv("TELEGRAM_BOT_TOKEN", "dummy_token")
	os.Setenv("TELEGRAM_CHAT_ID", "12345,invalid,67890")
	defer os.Unsetenv("TELEGRAM_BOT_TOKEN")
	defer os.Unsetenv("TELEGRAM_CHAT_ID")

	origNewTelegramNotifier := newTelegramNotifier
	newTelegramNotifier = func(token string, chatID int64, whitelist []int64) (Notifier, error) {
		return nil, errors.New("mock error")
	}
	defer func() { newTelegramNotifier = origNewTelegramNotifier }()

	cfg := &config.Config{}
	runner := New(cfg, nil, nil, nil)

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
