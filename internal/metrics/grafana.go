package metrics

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

// Client handles metrics shipping to Grafana Cloud.
type Client struct {
	apiKey   string
	endpoint string
	client   *http.Client
	enabled  bool
}

// Config holds Grafana Cloud configuration.
type Config struct {
	APIKey   string
	Endpoint string // https://prometheus-prod-XX-grafana.grafana.net/api/prom/push
}

// New creates a new Grafana metrics client.
func New(cfg Config) *Client {
	if cfg.APIKey == "" {
		return &Client{enabled: false}
	}

	return &Client{
		apiKey:   cfg.APIKey,
		endpoint: cfg.Endpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
		enabled:  true,
	}
}

// RecordJobCompletion records metrics for a completed job.
func (c *Client) RecordJobCompletion(result *domain.JobResult) error {
	if !c.enabled {
		return nil
	}

	metrics := fmt.Sprintf(`
# HELP hourglass_rejeicoes_total Total de rejeições
# TYPE hourglass_rejeicoes_total gauge
hourglass_rejeicoes_total{secao="%s"} %d

# HELP hourglass_job_duration_seconds Duração do job em segundos
# TYPE hourglass_job_duration_seconds histogram
hourglass_job_duration_seconds{secao="%s"} %f
`, result.Secao, result.Total, result.Secao, result.Duration.Seconds())

	return c.pushMetrics(metrics)
}

// RecordDailyStats records daily statistics.
func (c *Client) RecordDailyStats(stats *domain.DailyStats) error {
	if !c.enabled {
		return nil
	}

	metrics := fmt.Sprintf(`
# HELP hourglass_daily_jobs_total Total de jobs executados no dia
# TYPE hourglass_daily_jobs_total gauge
hourglass_daily_jobs_total %d

# HELP hourglass_daily_rejeicoes_total Total de rejeições no dia
# TYPE hourglass_daily_rejeicoes_total gauge
hourglass_daily_rejeicoes_total %d
`, stats.TotalJobs, stats.TotalRej)

	// Add per-section metrics
	for section, count := range stats.Sections {
		metrics += fmt.Sprintf(`
# HELP hourglass_section_rejeicoes_total Rejeições por seção
# TYPE hourglass_section_rejeicoes_total gauge
hourglass_section_rejeicoes_total{secao="%s"} %d
`, section, count)
	}

	return c.pushMetrics(metrics)
}

// pushMetrics sends metrics to Grafana Cloud.
func (c *Client) pushMetrics(metrics string) error {
	if c.endpoint == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", c.endpoint, bytes.NewBufferString(metrics))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to push metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grafana API returned status %d", resp.StatusCode)
	}

	return nil
}

// IsEnabled returns true if metrics shipping is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}
