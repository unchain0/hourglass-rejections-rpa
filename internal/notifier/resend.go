package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

const resendAPIURL = "https://api.resend.com/emails"

// ResendNotifier implements domain.Notifier using Resend API.
type ResendNotifier struct {
	apiKey string
	from   string
	to     string
	client *http.Client
}

// NewResendNotifier creates a new ResendNotifier.
func NewResendNotifier(apiKey, from, to string) *ResendNotifier {
	return &ResendNotifier{
		apiKey: apiKey,
		from:   from,
		to:     to,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// SendJobCompletion sends a summary email when a job completes.
func (r *ResendNotifier) SendJobCompletion(summary string, duration time.Duration) error {
	subject := "✅ Hourglass RPA - Análise Concluída"
	body := fmt.Sprintf(`
<h1>Análise Concluída ✅</h1>

<p><strong>Resumo:</strong> %s</p>
<p><strong>Duração:</u003e/strong> %s</p>
<p><strong>Timestamp:</u003e/strong> %s</p>

<hr>
<p>Hourglass Rejections RPA</p>
`, summary, duration, time.Now().Format(time.RFC3339))

	return r.sendEmail(subject, body)
}

// SendJobFailure sends an alert email when a job fails.
func (r *ResendNotifier) SendJobFailure(step string, err error) error {
	subject := "❌ Hourglass RPA - Falha na Execução"
	body := fmt.Sprintf(`
<h1>Falha na Execução ❌</h1>

<p><strong>Etapa:</u003e/strong> %s</p>
<p><strong>Erro:</u003e/strong> %s</p>
<p><strong>Timestamp:</u003e/strong> %s</p>

<hr>
<p>Hourglass Rejections RPA</p>
`, step, err.Error(), time.Now().Format(time.RFC3339))

	return r.sendEmail(subject, body)
}

// SendDailyReport sends a daily summary report.
func (r *ResendNotifier) SendDailyReport(stats domain.DailyStats) error {
	// Build sections summary
	sectionsHTML := ""
	for section, count := range stats.Sections {
		sectionsHTML += fmt.Sprintf("\u003cli><strong>%s:</strong> %d rejections</li>\n", section, count)
	}

	subject := "📊 Hourglass RPA - Relatório Diário"
	body := fmt.Sprintf(`
<h1>Relatório Diário 📊</h1>

<p><strong>Date:</u003e/strong> %s</p>
<p><strong>Total Jobs:</u003e/strong> %d</p>
<p><strong>Total Rejections:</u003e/strong> %d</p>

<h2>By Section:</h2>
<ul>
%s
</ul>

<hr>
<p>Hourglass Rejections RPA</p>
`, stats.Date.Format("2006-01-02"), stats.TotalJobs, stats.TotalRej, sectionsHTML)

	return r.sendEmail(subject, body)
}

// sendEmail sends an email via Resend API.
func (r *ResendNotifier) sendEmail(subject, htmlBody string) error {
	payload := map[string]string{
		"from":    r.from,
		"to":      r.to,
		"subject": subject,
		"html":    htmlBody,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", resendAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("resend API returned status %d", resp.StatusCode)
	}

	return nil
}
