package scheduler

import (
	"errors"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/cache"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/sentry"
)

func TestScheduler_SetNotifier(t *testing.T) {
	cfg := &config.Config{}
	sentryClient := &sentry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, sentryClient, analyzer, store)

	mockNotifier := &mockNotifier{}
	s.SetNotifier(mockNotifier)

	if s.notifier != mockNotifier {
		t.Error("SetNotifier should set the notifier field")
	}
}

func TestScheduler_sendNotifications_WithNotifier(t *testing.T) {
	mockNotifier := &mockNotifier{}
	s := &Scheduler{
		cache:    cache.New(),
		notifier: mockNotifier,
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err != nil {
		t.Errorf("sendNotifications should not return error, got: %v", err)
	}

	if !mockNotifier.sendJobCompletionCalled {
		t.Error("SendJobCompletion should have been called")
	}
}

func TestScheduler_sendNotifications_NoNotifier(t *testing.T) {
	s := &Scheduler{
		cache:    cache.New(),
		notifier: nil,
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err != nil {
		t.Errorf("sendNotifications should not return error when notifier is nil, got: %v", err)
	}
}

func TestScheduler_sendNotifications_NotifierError(t *testing.T) {
	mockNotifier := &mockNotifier{sendJobCompletionError: errors.New("notification failed")}
	s := &Scheduler{
		cache:        cache.New(),
		notifier:     mockNotifier,
		sentryClient: &sentry.Client{},
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err == nil {
		t.Error("sendNotifications should return error when notifier fails")
	}
}

func TestScheduler_sendNotifications_MultipleSections(t *testing.T) {
	mockNotifier := &mockNotifier{}
	s := &Scheduler{
		cache:    cache.New(),
		notifier: mockNotifier,
	}

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
		{Secao: "Partes Mecânicas", Quem: "Jane", OQue: "Video", PraQuando: "02/03/2026"},
		{Secao: "Campo", Quem: "Bob", OQue: "Console", PraQuando: "03/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err != nil {
		t.Errorf("sendNotifications should not return error, got: %v", err)
	}

	if !mockNotifier.sendJobCompletionCalled {
		t.Error("SendJobCompletion should have been called")
	}
}

type mockNotifier struct {
	sendJobCompletionCalled bool
	sendJobCompletionError  error
}

func (m *mockNotifier) SendJobCompletion(summary string, duration time.Duration) error {
	m.sendJobCompletionCalled = true
	return m.sendJobCompletionError
}

func (m *mockNotifier) SendJobFailure(step string, err error) error {
	return nil
}

func (m *mockNotifier) SendDailyReport(stats domain.DailyStats) error {
	return nil
}
