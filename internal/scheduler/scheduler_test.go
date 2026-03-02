package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/cache"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/sentry"
)

type mockAnalyzer struct {
	results map[string]*domain.JobResult
	err     error
}

func (m *mockAnalyzer) AnalyzeSection(section string) (*domain.JobResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if result, ok := m.results[section]; ok {
		return result, nil
	}
	return &domain.JobResult{Secao: section, Total: 0}, nil
}

type mockStorage struct {
	saved []domain.Rejeicao
	err   error
}

func (m *mockStorage) Save(ctx context.Context, rejections []domain.Rejeicao) error {
	if m.err != nil {
		return m.err
	}
	m.saved = append(m.saved, rejections...)
	return nil
}

func TestNew(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.cfg != cfg {
		t.Error("scheduler cfg not set correctly")
	}
	if s.sentryClient != sentryClient {
		t.Error("scheduler sentryClient not set correctly")
	}
	if s.analyzer != analyzer {
		t.Error("scheduler analyzer not set correctly")
	}
	if s.store != store {
		t.Error("scheduler store not set correctly")
	}
	if s.cache == nil {
		t.Error("scheduler cache should be initialized")
	}
}

func TestScheduler_sendNotifications_EmptyRejections(t *testing.T) {
	s := &Scheduler{
		cache: cache.New(),
	}

	err := s.sendNotifications([]domain.Rejeicao{})
	if err != nil {
		t.Errorf("sendNotifications with empty rejections should return nil, got: %v", err)
	}
}

func TestScheduler_sendNotifications_WithChanges(t *testing.T) {
	s := &Scheduler{
		cache: cache.New(),
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	err := s.sendNotifications(rejections)
	if err != nil {
		t.Errorf("sendNotifications with changes should return nil, got: %v", err)
	}
}

func TestScheduler_sendNotifications_NoChanges(t *testing.T) {
	s := &Scheduler{
		cache: cache.New(),
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	s.cache.HasChanges(rejections)
	err := s.sendNotifications(rejections)

	if err != nil {
		t.Errorf("sendNotifications with no changes should return nil, got: %v", err)
	}
}

func TestScheduler_Run_ContextCancellation(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if err != nil {
		t.Errorf("Run should return nil on context cancellation, got: %v", err)
	}
}

func TestScheduler_Run_BusinessHours(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if err != nil {
		t.Errorf("Run should return nil, got: %v", err)
	}
}

func TestScheduler_Run_NightHours(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if err != nil {
		t.Errorf("Run should return nil, got: %v", err)
	}
}

func TestScheduler_runAnalysis_AnalyzerError(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{err: errors.New("analyzer error")}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ctx := context.Background()
	err := s.runAnalysis(ctx)

	if err != nil {
		t.Errorf("runAnalysis should handle analyzer errors gracefully, got: %v", err)
	}
}

func TestScheduler_runAnalysis_ResultError(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{
		results: map[string]*domain.JobResult{
			"Partes Mecânicas": {Secao: "Partes Mecânicas", Error: errors.New("result error")},
		},
	}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ctx := context.Background()
	err := s.runAnalysis(ctx)

	if err != nil {
		t.Errorf("runAnalysis should handle result errors gracefully, got: %v", err)
	}
}

func TestScheduler_runAnalysis_WithRejections(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{
		results: map[string]*domain.JobResult{
			"Campo": {
				Secao:     "Campo",
				Total:     1,
				Rejeicoes: []domain.Rejeicao{{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"}},
			},
		},
	}
	mockStore := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, mockStore)

	ctx := context.Background()
	err := s.runAnalysis(ctx)

	if err != nil {
		t.Errorf("runAnalysis should succeed, got: %v", err)
	}

	if len(mockStore.saved) != 1 {
		t.Errorf("expected 1 rejection to be saved, got %d", len(mockStore.saved))
	}
}

func TestScheduler_runAnalysis_NoRejections(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{
		results: map[string]*domain.JobResult{
			"Campo": {Secao: "Campo", Total: 0, Rejeicoes: []domain.Rejeicao{}},
		},
	}
	mockStore := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, mockStore)

	ctx := context.Background()
	err := s.runAnalysis(ctx)

	if err != nil {
		t.Errorf("runAnalysis should succeed, got: %v", err)
	}

	if len(mockStore.saved) != 0 {
		t.Errorf("expected 0 rejections to be saved, got %d", len(mockStore.saved))
	}
}

func TestScheduler_runAnalysis_MultipleSections(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{
		results: map[string]*domain.JobResult{
			"Campo": {
				Secao:     "Campo",
				Total:     1,
				Rejeicoes: []domain.Rejeicao{{Secao: "Campo", Quem: "John", OQue: "Test1", PraQuando: "01/03/2026"}},
			},
			"Partes Mecânicas": {
				Secao:     "Partes Mecânicas",
				Total:     1,
				Rejeicoes: []domain.Rejeicao{{Secao: "Partes Mecânicas", Quem: "Jane", OQue: "Test2", PraQuando: "02/03/2026"}},
			},
		},
	}
	mockStore := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, mockStore)

	ctx := context.Background()
	err := s.runAnalysis(ctx)

	if err != nil {
		t.Errorf("runAnalysis should succeed, got: %v", err)
	}

	if len(mockStore.saved) != 2 {
		t.Errorf("expected 2 rejections to be saved, got %d", len(mockStore.saved))
	}
}

func TestScheduler_runAnalysis_StorageError(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{
		results: map[string]*domain.JobResult{
			"Campo": {
				Secao:     "Campo",
				Total:     1,
				Rejeicoes: []domain.Rejeicao{{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"}},
			},
		},
	}
	store := &mockStorage{err: errors.New("storage error")}

	s := New(cfg, sentryClient, analyzer, store)

	ctx := context.Background()
	err := s.runAnalysis(ctx)

	if err != nil {
		t.Errorf("runAnalysis should handle storage errors gracefully, got: %v", err)
	}
}

func TestScheduler_calculateInterval_BusinessHours(t *testing.T) {
	s := &Scheduler{}

	businessTimes := []int{6, 12, 15, 21}
	for _, hour := range businessTimes {
		now := time.Date(2026, 3, 2, hour, 0, 0, 0, time.UTC)
		interval := s.calculateInterval(now)
		if interval != 30*time.Minute {
			t.Errorf("Hour %d: expected 30m interval, got %v", hour, interval)
		}
	}
}

func TestScheduler_calculateInterval_NightHours(t *testing.T) {
	s := &Scheduler{}

	nightTimes := []int{0, 1, 5, 22, 23}
	for _, hour := range nightTimes {
		now := time.Date(2026, 3, 2, hour, 0, 0, 0, time.UTC)
		interval := s.calculateInterval(now)
		if interval != 2*time.Hour {
			t.Errorf("Hour %d: expected 2h interval, got %v", hour, interval)
		}
	}
}

func TestScheduler_runWithTicker_SingleTick(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := s.runWithTicker(ctx, ticker)
	if err != nil {
		t.Errorf("runWithTicker should return nil, got: %v", err)
	}
}

func TestScheduler_runWithTicker_MultipleTicks(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := s.runWithTicker(ctx, ticker)
	if err != nil {
		t.Errorf("runWithTicker should return nil, got: %v", err)
	}
}
