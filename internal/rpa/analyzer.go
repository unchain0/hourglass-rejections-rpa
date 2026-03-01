package rpa

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"

	"hourglass-rejeicoes-rpa/internal/domain"
)

// Analyzer handles the scraping of rejection data from Hourglass AG-Grid.
type Analyzer struct {
	browser      *Browser
	loginManager *LoginManager
}

// NewAnalyzer creates a new Analyzer instance.
func NewAnalyzer(browser *Browser, loginManager *LoginManager) *Analyzer {
	return &Analyzer{
		browser:      browser,
		loginManager: loginManager,
	}
}

// AnalyzeSection scrapes rejection data from a specific section.
// Supported sections: "Partes Mecânicas", "Campo", "Testemunho Público"
func (a *Analyzer) AnalyzeSection(ctx context.Context, section string) (*domain.JobResult, error) {
	start := time.Now()

	// Check if authenticated
	authenticated, err := a.loginManager.IsAuthenticated(ctx)
	if err != nil {
		return &domain.JobResult{
			Secao:    section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("failed to check authentication: %w", err),
		}, nil
	}

	if !authenticated {
		return &domain.JobResult{
			Secao:    section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("not authenticated"),
		}, nil
	}

	// Navigate to the section
	if err := a.navigateToSection(ctx, section); err != nil {
		return &domain.JobResult{
			Secao:    section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("failed to navigate to section %s: %w", section, err),
		}, nil
	}

	// Extract data from AG-Grid
	rejeicoes, err := a.extractFromAGGrid(ctx, section)
	if err != nil {
		return &domain.JobResult{
			Secao:    section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("failed to extract data from grid: %w", err),
		}, nil
	}

	return &domain.JobResult{
		Secao:     section,
		Total:     len(rejeicoes),
		Rejeicoes: rejeicoes,
		Duration:  time.Since(start),
		Error:     nil,
	}, nil
}

// navigateToSection navigates to the specific section in Hourglass.
func (a *Analyzer) navigateToSection(ctx context.Context, section string) error {
	// Navigate to base URL first
	url := "https://hourglass.petrobras.com"

	err := chromedp.Run(a.browser.Context(),
		chromedp.Navigate(url),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("failed to navigate to Hourglass: %w", err)
	}

	// TODO: Implement section-specific navigation
	// This will depend on the actual Hourglass UI structure
	// Options:
	// 1. Click on a tab with the section name
	// 2. Select from a dropdown
	// 3. Navigate to a specific URL path

	return nil
}

// extractFromAGGrid extracts all rows from the AG-Grid and converts them to Rejeicao structs.
func (a *Analyzer) extractFromAGGrid(ctx context.Context, section string) ([]domain.Rejeicao, error) {
	var rejeicoes []domain.Rejeicao

	// Wait for AG-Grid to be visible
	err := chromedp.Run(a.browser.Context(),
		chromedp.WaitVisible(".ag-root", chromedp.ByQuery),
	)
	if err != nil {
		return nil, fmt.Errorf("AG-Grid not found: %w", err)
	}

	// Extract rows using JavaScript evaluation
	var rows []map[string]string
	err = chromedp.Run(a.browser.Context(),
		chromedp.Evaluate(`
			(function() {
				const rows = document.querySelectorAll('.ag-row');
				const data = [];
				rows.forEach(row => {
					const cells = row.querySelectorAll('.ag-cell');
					if (cells.length >= 3) {
						data.push({
							quem: cells[0]?.textContent?.trim() || '',
							oque: cells[1]?.textContent?.trim() || '',
							praQuando: cells[2]?.textContent?.trim() || ''
						});
					}
				});
				return data;
			})()
		`, &rows),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to extract grid data: %w", err)
	}

	// Convert extracted data to Rejeicao structs
	timestamp := time.Now()
	for _, row := range rows {
		rejeicoes = append(rejeicoes, domain.Rejeicao{
			Secao:     section,
			Quem:      row["quem"],
			OQue:      row["oque"],
			PraQuando: row["praQuando"],
			Timestamp: timestamp,
		})
	}

	return rejeicoes, nil
}

// GetSectionData is a convenience method to analyze a section and return the result.
func (a *Analyzer) GetSectionData(ctx context.Context, section string) (*domain.JobResult, error) {
	return a.AnalyzeSection(ctx, section)
}
