package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
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

func setupTestServer(t *testing.T, handler http.HandlerFunc) (*testServer, *ResendNotifier) {
	server := newHTTPTestServer(t, handler)
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
		Date:            time.Now(),
		TotalJobs:       6,
		TotalRejections: 42,
		Sections: map[string]int{
			"Mechanical Parts":  15,
			"Field Ministry":    18,
			"Public Witnessing": 9,
		},
	}

	err := n.SendDailyReport(stats)
	if err != nil {
		t.Errorf("SendDailyReport() error = %v", err)
	}
}

func TestResendNotifier_EmailHTMLIsValidAndEscaped(t *testing.T) {
	var bodies []string
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		bodies = append(bodies, payload["html"])
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	if err := n.SendJobCompletion("<script>alert(1)</script>", time.Second); err != nil {
		t.Fatalf("SendJobCompletion() error = %v", err)
	}
	if err := n.SendJobFailure("<step>", errors.New("<failure>")); err != nil {
		t.Fatalf("SendJobFailure() error = %v", err)
	}
	if err := n.SendDailyReport(domain.DailyStats{
		Date:     time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC),
		Sections: map[string]int{"<section>": 1},
	}); err != nil {
		t.Fatalf("SendDailyReport() error = %v", err)
	}

	if len(bodies) != 3 {
		t.Fatalf("got %d email bodies, want 3", len(bodies))
	}
	for _, body := range bodies {
		if strings.Contains(body, "u003e/strong") {
			t.Errorf("email contains malformed closing tag: %s", body)
		}
	}
	if !strings.Contains(bodies[0], "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("completion summary was not escaped: %s", bodies[0])
	}
	if !strings.Contains(bodies[1], "&lt;step&gt;") || !strings.Contains(bodies[1], "&lt;failure&gt;") {
		t.Errorf("failure fields were not escaped: %s", bodies[1])
	}
	if !strings.Contains(bodies[2], "<li><strong>&lt;section&gt;:</strong> 1 rejections</li>") {
		t.Errorf("daily report section is malformed or unescaped: %s", bodies[2])
	}
}

func TestResendNotifier_sendEmail_Status201(t *testing.T) {
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	defer server.Close()

	err := n.SendJobCompletion("Test", 1*time.Minute)
	if err != nil {
		t.Errorf("SendJobCompletion() with 201 status unexpected error = %v", err)
	}
}

func TestResendNotifier_sendEmail_Non200Status(t *testing.T) {
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	stats := domain.DailyStats{
		Date:            time.Now(),
		TotalJobs:       0,
		TotalRejections: 0,
		Sections:        map[string]int{},
	}

	err := n.SendDailyReport(stats)
	if err != nil {
		t.Errorf("SendDailyReport() with empty sections error = %v", err)
	}
}

func TestResendNotifier_SendJobCompletion_LongDuration(t *testing.T) {
	server, n := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := n.SendJobCompletion("Long job completed", 24*time.Hour+15*time.Minute)
	if err != nil {
		t.Errorf("SendJobCompletion() with long duration error = %v", err)
	}
}

func TestResendNotifier_sendEmail_MarshalError(t *testing.T) {
	original := jsonMarshal
	defer func() { jsonMarshal = original }()

	jsonMarshal = func(_ any) ([]byte, error) {
		return nil, errors.New("marshal error")
	}

	n := NewResendNotifier("test-api-key", "from@example.com", "to@example.com")
	err := n.sendEmail("Test Subject", "<h1>Test</h1>")
	if err == nil {
		t.Fatal("expected error for marshal failure")
	}
	if err.Error() != "failed to marshal email payload: marshal error" {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestResendNotifier_sendEmail_RestoresMarshal(t *testing.T) {
	_ = json.Marshal
	original := jsonMarshal
	defer func() { jsonMarshal = original }()

	jsonMarshal = func(_ any) ([]byte, error) {
		return nil, errors.New("fail")
	}
	n := NewResendNotifier("k", "f", "t")
	_ = n.sendEmail("s", "b")

	jsonMarshal = original
	server, n2 := setupTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := n2.sendEmail("s", "b")
	if err != nil {
		t.Errorf("expected no error after restoring marshal, got %v", err)
	}
}

func TestResendNotifier_sendEmail_NewRequestError(t *testing.T) {
	original := httpNewRequest
	defer func() { httpNewRequest = original }()

	httpNewRequest = func(_ context.Context, _ string, _ string, _ io.Reader) (*http.Request, error) {
		return nil, errors.New("request creation error")
	}

	n := NewResendNotifier("test-api-key", "from@example.com", "to@example.com")
	err := n.sendEmail("Test Subject", "<h1>Test</h1>")
	if err == nil {
		t.Fatal("expected error for request creation failure")
	}
	if err.Error() != "failed to create request: request creation error" {
		t.Errorf("unexpected error: %s", err.Error())
	}
}
