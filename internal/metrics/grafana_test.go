package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

func TestNew_Disabled(t *testing.T) {
	cfg := Config{
		APIKey:   "",
		Endpoint: "https://example.com",
	}

	client := New(cfg)
	if client == nil {
		t.Fatal("New() returned nil")
	}

	if client.IsEnabled() {
		t.Error("expected client to be disabled when APIKey is empty")
	}
}

func TestNew_Enabled(t *testing.T) {
	cfg := Config{
		APIKey:   "test-api-key",
		Endpoint: "https://example.com",
	}

	client := New(cfg)
	if client == nil {
		t.Fatal("New() returned nil")
	}

	if !client.IsEnabled() {
		t.Error("expected client to be enabled when APIKey is set")
	}
}

func TestClient_RecordJobCompletion_Disabled(t *testing.T) {
	client := &Client{enabled: false}

	result := &domain.JobResult{
		Secao:     "Test Section",
		Total:     10,
		Duration:  5 * time.Minute,
		Rejeicoes: []domain.Rejeicao{},
	}

	// Should not error when disabled
	err := client.RecordJobCompletion(result)
	if err != nil {
		t.Errorf("RecordJobCompletion() error = %v", err)
	}
}

func TestClient_RecordDailyStats_Disabled(t *testing.T) {
	client := &Client{enabled: false}

	stats := &domain.DailyStats{
		Date:      time.Now(),
		TotalJobs: 6,
		TotalRej:  42,
		Sections: map[string]int{
			"Section1": 20,
			"Section2": 22,
		},
	}

	// Should not error when disabled
	err := client.RecordDailyStats(stats)
	if err != nil {
		t.Errorf("RecordDailyStats() error = %v", err)
	}
}

func TestClient_pushMetrics_NoEndpoint(t *testing.T) {
	client := &Client{
		enabled:  true,
		endpoint: "",
	}

	// Should not error when endpoint is empty
	err := client.pushMetrics("test metrics")
	if err != nil {
		t.Errorf("pushMetrics() error = %v", err)
	}
}

func TestClient_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{enabled: tt.enabled}
			if got := c.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}


func TestClient_RecordJobCompletion_Enabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization header 'Bearer test-api-key', got '%s'", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("expected Content-Type 'text/plain', got '%s'", r.Header.Get("Content-Type"))
		}
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		apiKey:   "test-api-key",
		endpoint: server.URL,
		client:   &http.Client{},
		enabled:  true,
	}

	result := &domain.JobResult{
		Secao:     "Test Section",
		Total:     10,
		Duration:  5 * time.Minute,
		Rejeicoes: []domain.Rejeicao{},
	}

	err := client.RecordJobCompletion(result)
	if err != nil {
		t.Errorf("RecordJobCompletion() error = %v", err)
	}
}


func TestClient_RecordDailyStats_Enabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization header 'Bearer test-api-key', got '%s'", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("expected Content-Type 'text/plain', got '%s'", r.Header.Get("Content-Type"))
		}
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		apiKey:   "test-api-key",
		endpoint: server.URL,
		client:   &http.Client{},
		enabled:  true,
	}

	stats := &domain.DailyStats{
		Date:      time.Now(),
		TotalJobs: 6,
		TotalRej: 42,
		Sections: map[string]int{
			"Section1": 20,
			"Section2": 22,
		},
	}

	err := client.RecordDailyStats(stats)
	if err != nil {
		t.Errorf("RecordDailyStats() error = %v", err)
	}
}


func TestClient_pushMetrics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization header 'Bearer test-api-key', got '%s'", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("expected Content-Type 'text/plain', got '%s'", r.Header.Get("Content-Type"))
		}
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &Client{
		apiKey:   "test-api-key",
		endpoint: server.URL,
		client:   &http.Client{},
		enabled:  true,
	}

	metrics := "test metrics data"
	err := client.pushMetrics(metrics)
	if err != nil {
		t.Errorf("pushMetrics() error = %v", err)
	}
}

func TestClient_pushMetrics_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := &Client{
		apiKey:   "test-api-key",
		endpoint: server.URL,
		client:   &http.Client{},
		enabled:  true,
	}

	metrics := "test metrics data"
	err := client.pushMetrics(metrics)
	if err == nil {
		t.Error("pushMetrics() expected error for non-200 status, got nil")
	}
	expectedMsg := "grafana API returned status 400"
	if err.Error() != expectedMsg {
		t.Errorf("pushMetrics() error = %v, want %v", err.Error(), expectedMsg)
	}
}

func TestClient_pushMetrics_HTTPError(t *testing.T) {
	// Use an invalid URL that will cause an HTTP error
	client := &Client{
		apiKey:   "test-api-key",
		endpoint: "http://invalid-url-that-does-not-exist-12345.com",
		client:   &http.Client{Timeout: 1 * time.Second},
		enabled:  true,
	}

	metrics := "test metrics data"
	err := client.pushMetrics(metrics)
	if err == nil {
		t.Error("pushMetrics() expected error for invalid URL, got nil")
	}
	if !containsSubstring(err.Error(), "failed to push metrics") {
		t.Errorf("pushMetrics() error = %v, want substring 'failed to push metrics'", err.Error())
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}


func TestClient_pushMetrics_CreateRequestError(t *testing.T) {
	// Use an invalid URL that causes NewRequestWithContext to fail
	// Control character in URL will cause URL parsing error at request creation
	client := &Client{
		apiKey:   "test-api-key",
		endpoint: "http://example.com\x00invalid", // Contains null byte which will cause URL parse error
		client:   &http.Client{},
		enabled:  true,
	}

	metrics := "test metrics data"
	err := client.pushMetrics(metrics)
	if err == nil {
		t.Error("pushMetrics() expected error for invalid URL, got nil")
	}
	expectedMsg := "failed to create request"
	if !containsSubstring(err.Error(), expectedMsg) {
		t.Errorf("pushMetrics() error = %v, want substring '%s'", err.Error(), expectedMsg)
	}
}
