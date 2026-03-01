package notifier

import (
	"errors"
	"testing"
	"time"

	"hourglass-rejeicoes-rpa/internal/domain"
)

func TestNewResendNotifier(t *testing.T) {
	apiKey := "test-api-key"
	from := "from@example.com"
	to := "to@example.com"

	n := NewResendNotifier(apiKey, from, to)

	if n == nil {
		t.Fatal("NewResendNotifier returned nil")
	}

	if n.apiKey != apiKey {
		t.Error("apiKey not set correctly")
	}

	if n.from != from {
		t.Error("from not set correctly")
	}

	if n.to != to {
		t.Error("to not set correctly")
	}

	if n.client == nil {
		t.Error("http client not initialized")
	}
}

func TestResendNotifier_sendEmail_InvalidAPIKey(t *testing.T) {
	// This test would fail with real API - skip for unit tests
	t.Skip("Skipping test that requires real Resend API")

	n := NewResendNotifier("invalid-key", "from@example.com", "to@example.com")

	err := n.sendEmail("Test Subject", "<h1>Test</h1>")
	if err == nil {
		t.Error("expected error for invalid API key")
	}
}

func TestResendNotifier_SendJobCompletion(t *testing.T) {
	t.Skip("Skipping test that requires real Resend API")

	n := NewResendNotifier("test-key", "from@example.com", "to@example.com")

	err := n.SendJobCompletion("Test summary", 5*time.Minute)
	if err != nil {
		t.Errorf("SendJobCompletion() error = %v", err)
	}
}

func TestResendNotifier_SendJobFailure(t *testing.T) {
	t.Skip("Skipping test that requires real Resend API")

	n := NewResendNotifier("test-key", "from@example.com", "to@example.com")

	testErr := errors.New("test error")
	err := n.SendJobFailure("test-step", testErr)
	if err != nil {
		t.Errorf("SendJobFailure() error = %v", err)
	}
}

func TestResendNotifier_SendDailyReport(t *testing.T) {
	t.Skip("Skipping test that requires real Resend API")

	n := NewResendNotifier("test-key", "from@example.com", "to@example.com")

	stats := domain.DailyStats{
		Date:      time.Now(),
		TotalJobs: 6,
		TotalRej:  42,
		Sections: map[string]int{
			"Partes Mecânicas":   15,
			"Campo":              18,
			"Testemunho Público": 9,
		},
	}

	err := n.SendDailyReport(stats)
	if err != nil {
		t.Errorf("SendDailyReport() error = %v", err)
	}
}
