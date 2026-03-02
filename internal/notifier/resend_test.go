package notifier

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

// mockTransport redirects requests to the test server
type mockTransport struct {
	targetURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Create a new request with the target URL
	newURL, err := url.Parse(m.targetURL)
	if err != nil {
		return nil, err
	}

	req.URL.Scheme = newURL.Scheme
	req.URL.Host = newURL.Host

	// Use the default transport to actually make the request
	return http.DefaultTransport.RoundTrip(req)
}

// failingTransport is a custom RoundTripper that always fails
type failingTransport struct{}

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network error")
}

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

func setupTestServer(handler http.HandlerFunc) (*httptest.Server, *ResendNotifier) {
	server := httptest.NewServer(handler)
	transport := &mockTransport{targetURL: server.URL}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	n := &ResendNotifier{
		apiKey: "test-api-key",
		from:   "from@example.com",
		to:     "to@example.com",
		client: client,
	}
	return server, n
}

func TestResendNotifier_sendEmail_Success200(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		// Verify Content-Type header
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type header to be application/json")
		}

		// Verify Authorization header
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("expected Authorization header with Bearer token")
		}

		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := n.sendEmail("Test Subject", "<h1>Test Body</h1>")
	if err != nil {
		t.Errorf("sendEmail() unexpected error = %v", err)
	}
}

func TestResendNotifier_SendJobCompletion_Success(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request contains expected fields
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("expected Authorization header")
		}

		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := n.SendJobCompletion("Job completed successfully", 5*time.Minute)
	if err != nil {
		t.Errorf("SendJobCompletion() error = %v", err)
	}
}

func TestResendNotifier_SendJobFailure_Success(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("expected Authorization header")
		}

		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	testErr := errors.New("test error occurred")
	err := n.SendJobFailure("test-step", testErr)
	if err != nil {
		t.Errorf("SendJobFailure() error = %v", err)
	}
}

func TestResendNotifier_SendDailyReport_Success(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("expected Authorization header")
		}

		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

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

func TestResendNotifier_sendEmail_Status201(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	defer server.Close()

	err := n.SendJobCompletion("Test", 1*time.Minute)
	if err != nil {
		t.Errorf("SendJobCompletion() with 201 status unexpected error = %v", err)
	}
}

func TestResendNotifier_sendEmail_Non200Status(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	err := n.SendJobCompletion("Test", 1*time.Minute)
	if err == nil {
		t.Error("expected error for non-200 status")
	}

	expectedErr := "resend API returned status 400"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestResendNotifier_sendEmail_Unauthorized(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer server.Close()

	err := n.SendJobCompletion("Test", 1*time.Minute)
	if err == nil {
		t.Error("expected error for 401 status")
	}

	expectedErr := "resend API returned status 401"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestResendNotifier_sendEmail_InternalServerError(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()

	err := n.SendJobCompletion("Test", 1*time.Minute)
	if err == nil {
		t.Error("expected error for 500 status")
	}

	expectedErr := "resend API returned status 500"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestResendNotifier_sendEmail_HTTPRequestFailure(t *testing.T) {
	n := NewResendNotifier("test-api-key", "from@example.com", "to@example.com")
	n.client.Transport = &failingTransport{}

	err := n.SendJobCompletion("Test", 1*time.Minute)
	if err == nil {
		t.Error("expected error for HTTP request failure")
	}

	expectedErr := "failed to send email"
	if err == nil || err.Error()[:len(expectedErr)] != expectedErr {
		t.Errorf("expected error to contain '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestResendNotifier_SendDailyReport_EmptySections(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	stats := domain.DailyStats{
		Date:      time.Now(),
		TotalJobs: 0,
		TotalRej:  0,
		Sections:  map[string]int{},
	}

	err := n.SendDailyReport(stats)
	if err != nil {
		t.Errorf("SendDailyReport() with empty sections error = %v", err)
	}
}

func TestResendNotifier_SendJobCompletion_LongDuration(t *testing.T) {
	server, n := setupTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := n.SendJobCompletion("Long job completed", 24*time.Hour+15*time.Minute)
	if err != nil {
		t.Errorf("SendJobCompletion() with long duration error = %v", err)
	}
}
