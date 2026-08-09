package scheduler

import (
	"errors"
	"testing"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/engines/rejection_cache"
	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"
)

func TestScheduler_SetNotifier(t *testing.T) {
	cfg := &config.Config{}
	telemetryClient := &telemetry.Client{}
	analyzer := &mockAnalyzer{}
	store := &mockStorage{}

	s := New(cfg, telemetryClient, analyzer, store)

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

	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err != nil {
		t.Errorf("sendNotifications should not return error, got: %v", err)
	}

	if !mockNotifier.sendRejectionsCalled {
		t.Error("SendRejections should have been called")
	}
	if len(mockNotifier.rejections) != 1 || mockNotifier.rejections[0] != rejections[0] {
		t.Fatalf("SendRejections received %v, want %v", mockNotifier.rejections, rejections)
	}
}

func TestScheduler_sendNotifications_NoNotifier(t *testing.T) {
	s := &Scheduler{
		cache:    cache.New(),
		notifier: nil,
	}

	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err != nil {
		t.Errorf("sendNotifications should not return error when notifier is nil, got: %v", err)
	}

	mockNotifier := &mockNotifier{}
	s.SetNotifier(mockNotifier)
	if err := s.sendNotifications(rejections, time.Second); err != nil {
		t.Fatalf("same snapshot should send after notifier is configured: %v", err)
	}
	if mockNotifier.sendRejectionsCalls != 1 {
		t.Fatalf("SendRejections called %d times, want 1", mockNotifier.sendRejectionsCalls)
	}
}

func TestScheduler_sendNotifications_NotifierError(t *testing.T) {
	mockNotifier := &mockNotifier{sendRejectionsError: errors.New("notification failed")}
	s := &Scheduler{
		cache:           cache.New(),
		notifier:        mockNotifier,
		telemetryClient: &telemetry.Client{},
	}

	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err == nil {
		t.Error("sendNotifications should return error when notifier fails")
	}
}

func TestScheduler_sendNotifications_RetriesAfterNotifierError(t *testing.T) {
	mockNotifier := &mockNotifier{sendRejectionsError: errors.New("notification failed")}
	s := &Scheduler{
		cache:           cache.New(),
		notifier:        mockNotifier,
		telemetryClient: &telemetry.Client{},
	}
	rejections := []domain.Rejection{{Section: "Field Ministry", Who: "John", What: "Test"}}

	firstErr := s.sendNotifications(rejections, time.Second)
	secondErr := s.sendNotifications(rejections, time.Second)

	if firstErr == nil || secondErr == nil {
		t.Fatalf("both delivery attempts should fail, got first=%v second=%v", firstErr, secondErr)
	}
	if mockNotifier.sendRejectionsCalls != 2 {
		t.Fatalf("SendRejections called %d times, want 2", mockNotifier.sendRejectionsCalls)
	}
}

func TestScheduler_sendNotifications_MultipleSections(t *testing.T) {
	mockNotifier := &mockNotifier{}
	s := &Scheduler{
		cache:    cache.New(),
		notifier: mockNotifier,
	}

	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
		{Section: "Mechanical Parts", Who: "Jane", What: "Video", When: "02/03/2026"},
		{Section: "Field Ministry", Who: "Bob", What: "Console", When: "03/03/2026"},
	}

	err := s.sendNotifications(rejections, time.Second)
	if err != nil {
		t.Errorf("sendNotifications should not return error, got: %v", err)
	}

	if !mockNotifier.sendRejectionsCalled {
		t.Error("SendRejections should have been called")
	}
}

type mockNotifier struct {
	sendRejectionsCalled bool
	sendRejectionsCalls  int
	sendRejectionsError  error
	rejections           []domain.Rejection
}

func (m *mockNotifier) SendRejections(rejections []domain.Rejection) error {
	m.sendRejectionsCalled = true
	m.sendRejectionsCalls++
	m.rejections = append([]domain.Rejection(nil), rejections...)
	return m.sendRejectionsError
}
