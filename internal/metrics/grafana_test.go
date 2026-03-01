package metrics

import (
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
