package api

import (
	"fmt"
	"strings"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

// APIAnalyzer uses the Hourglass REST API to detect rejections.
type APIAnalyzer struct {
	client         *Client
	userCache      map[int]*User
	congregationID int
	daysToLookAhead int
}

// NewAPIAnalyzer creates a new API-based analyzer.
func NewAPIAnalyzer(client *Client) *APIAnalyzer {
	return &APIAnalyzer{
		client:         client,
		userCache:      make(map[int]*User),
		congregationID: 48092, // Default congregation ID
		daysToLookAhead: 730,     // Default: 2 years (730 days) to catch all rejections
	}
}

// SetCongregationID sets the congregation ID for API calls.
func (a *APIAnalyzer) SetCongregationID(id int) {
	a.congregationID = id
}

// SetDaysToLookAhead sets the number of days to look ahead for rejections.
func (a *APIAnalyzer) SetDaysToLookAhead(days int) {
	a.daysToLookAhead = days
}

// AnalyzeSection analyzes a specific section for rejections.
// Supported sections: "Partes Mecânicas", "Campo", "Testemunho Público"
func (a *APIAnalyzer) AnalyzeSection(section string) (*domain.JobResult, error) {
	start := time.Now()

	// Load users cache if empty
	if len(a.userCache) == 0 {
		if err := a.loadUsers(); err != nil {
			return &domain.JobResult{
				Secao:    section,
				Duration: time.Since(start),
				Error:    fmt.Errorf("failed to load users: %w", err),
			}, nil
		}
	}

	var rejeicoes []domain.Rejeicao
	var err error

	switch section {
	case "Partes Mecânicas", "avattendant":
		rejeicoes, err = a.analyzePartesMecanicas()
	case "Campo", "fsMeeting":
		rejeicoes, err = a.analyzeCampo()
	case "Testemunho Público", "publicWitnessing":
		rejeicoes, err = a.analyzeTestemunhoPublico()
	default:
		return &domain.JobResult{
			Secao:    section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("unknown section: %s", section),
		}, nil
	}

	if err != nil {
		return &domain.JobResult{
			Secao:    section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("failed to analyze section %s: %w", section, err),
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

// loadUsers loads all users into the cache.
func (a *APIAnalyzer) loadUsers() error {
	users, err := a.client.GetUsers()
	if err != nil {
		return err
	}

	for i := range users {
		a.userCache[users[i].ID] = &users[i]
	}

	return nil
}

// getUserName returns the user's display name from cache.
func (a *APIAnalyzer) getUserName(userID int) string {
	if user, ok := a.userCache[userID]; ok {
		if user.Descriptor != "" {
			return user.Descriptor
		}
		return fmt.Sprintf("%s %s", user.Firstname, user.Lastname)
	}
	return fmt.Sprintf("User %d", userID)
}

// analyzePartesMecanicas analyzes mechanical assignments for rejections.
// Uses notifications endpoint to detect declined assignments.
func (a *APIAnalyzer) analyzePartesMecanicas() ([]domain.Rejeicao, error) {
	start := time.Now().Format("2006-01-02")
	end := time.Now().AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	// Use notifications endpoint with type "ava"
	notifications, err := a.client.GetNotifications(start, end, "ava")
	if err != nil {
		return nil, err
	}

	var rejeicoes []domain.Rejeicao
	timestamp := time.Now()

	for _, notif := range notifications {
		// Only detect DECLINED status (recusado)
		if notif.Status == "declined" {
			rejeicoes = append(rejeicoes, domain.Rejeicao{
				Secao:     "Partes Mecânicas",
				Quem:      a.getUserName(notif.Assignee),
				OQue:      getFriendlyTypeName(notif.Type),
				PraQuando: formatDateToBrazilian(notif.Date),
				Timestamp: timestamp,
			})
		}
	}

	return rejeicoes, nil
}
// formatDateToBrazilian converts YYYY-MM-DD to DD/MM/YYYY
func formatDateToBrazilian(date string) string {
	parts := strings.Split(date, "-")
	if len(parts) != 3 {
		return date
	}
	return fmt.Sprintf("%s/%s/%s", parts[2], parts[1], parts[0])
}

// getFriendlyTypeName converts technical type names to user-friendly names
func getFriendlyTypeName(typeName string) string {
	switch typeName {
	case "ava":
		return "Áudio/Vídeo & Indicadores"
	case "video":
		return "Vídeo"
	case "console":
		return "Console"
	case "mics":
		return "Microfone"
	case "attendant":
		return "Atendente"
	case "pubwit":
		return "Testemunho Público"
	case "fm":
		return "Reunião de Campo"
	default:
		return typeName
	}
}



// analyzeCampo analyzes field ministry assignments for rejections.
// Uses notifications endpoint to detect declined assignments.
func (a *APIAnalyzer) analyzeCampo() ([]domain.Rejeicao, error) {
	start := time.Now().Format("2006-01-02")
	end := time.Now().AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	// Use notifications endpoint with type "fm" (field ministry)
	notifications, err := a.client.GetNotifications(start, end, "fm")
	if err != nil {
		return nil, err
	}

	var rejeicoes []domain.Rejeicao
	timestamp := time.Now()

	for _, notif := range notifications {
		// Only detect DECLINED status (recusado)
		if notif.Status == "declined" {
			rejeicoes = append(rejeicoes, domain.Rejeicao{
				Secao:     "Campo",
				Quem:      a.getUserName(notif.Assignee),
				OQue:      getFriendlyTypeName(notif.Type),
				PraQuando: formatDateToBrazilian(notif.Date),
				Timestamp: timestamp,
			})
		}
	}

	return rejeicoes, nil
}

// analyzeTestemunhoPublico analyzes public witnessing assignments for rejections.
// Uses notifications endpoint to detect declined assignments.
func (a *APIAnalyzer) analyzeTestemunhoPublico() ([]domain.Rejeicao, error) {
	start := time.Now().Format("2006-01-02")
	end := time.Now().AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	// Use notifications endpoint with type "pubwit"
	notifications, err := a.client.GetNotifications(start, end, "pubwit")
	if err != nil {
		return nil, err
	}

	var rejeicoes []domain.Rejeicao
	timestamp := time.Now()

	for _, notif := range notifications {
		// Only detect DECLINED status (recusado)
		if notif.Status == "declined" {
			rejeicoes = append(rejeicoes, domain.Rejeicao{
				Secao:     "Testemunho Público",
				Quem:      a.getUserName(notif.Assignee),
				OQue:      getFriendlyTypeName(notif.Type),
				PraQuando: formatDateToBrazilian(notif.Date),
				Timestamp: timestamp,
			})
		}
	}

	return rejeicoes, nil
}

// AnalyzeAllSections analyzes all three sections.
func (a *APIAnalyzer) AnalyzeAllSections() ([]domain.JobResult, error) {
	sections := []string{"Partes Mecânicas", "Campo", "Testemunho Público"}
	var results []domain.JobResult

	for _, section := range sections {
		result, _ := a.AnalyzeSection(section)
		results = append(results, *result)
	}

	return results, nil
}
