package api

import (
	"fmt"
	"strings"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

// APIAnalyzer uses the Hourglass REST API to detect rejections.
type APIAnalyzer struct {
	client          *Client
	userCache       map[int]*User
	congregationID  int
	daysToLookAhead int
}

// NewAPIAnalyzer creates a new API-based analyzer.
func NewAPIAnalyzer(client *Client) *APIAnalyzer {
	return &APIAnalyzer{
		client:          client,
		userCache:       make(map[int]*User),
		congregationID:  48092, // Default congregation ID
		daysToLookAhead: 730,   // Default: 2 years (730 days) to catch all rejections
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
	case "Reunião Meio de Semana", "midweekMeeting":
		rejeicoes, err = a.analyzeMidweekMeetings()
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

// analyzeGenericNotifications is a generic function to analyze notifications for a section.
func (a *APIAnalyzer) analyzeGenericNotifications(sectionName, notificationType string) ([]domain.Rejeicao, error) {
	start := time.Now().Format("2006-01-02")
	end := time.Now().AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	notifications, err := a.client.GetNotifications(start, end, notificationType)
	if err != nil {
		return nil, err
	}

	var rejeicoes []domain.Rejeicao
	timestamp := time.Now()

	for _, notif := range notifications {
		if notif.Status == "declined" {
			rejeicoes = append(rejeicoes, domain.Rejeicao{
				Secao:     sectionName,
				Quem:      a.getUserName(notif.Assignee),
				OQue:      getFriendlyTypeName(notif.Type),
				PraQuando: formatDateToBrazilian(notif.Date),
				Timestamp: timestamp,
			})
		}
	}

	return rejeicoes, nil
}

// analyzePartesMecanicas analyzes mechanical assignments for rejections.
func (a *APIAnalyzer) analyzePartesMecanicas() ([]domain.Rejeicao, error) {
	return a.analyzeGenericNotifications("Partes Mecânicas", "ava")
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

func (a *APIAnalyzer) analyzeCampo() ([]domain.Rejeicao, error) {
	return a.analyzeGenericNotifications("Campo", "fm")
}

func (a *APIAnalyzer) analyzeTestemunhoPublico() ([]domain.Rejeicao, error) {
	return a.analyzeGenericNotifications("Testemunho Público", "pubwit")
}

// analyzeMidweekMeetings analyzes midweek meeting assignments for rejections.
func (a *APIAnalyzer) analyzeMidweekMeetings() ([]domain.Rejeicao, error) {
	start := time.Now().Format("2006-01-02")
	end := time.Now().AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	notifications, err := a.client.GetNotifications(start, end, "mm")
	if err != nil {
		return nil, err
	}

	// Buscar programa da reunião para obter títulos das partes
	meetings, err := a.client.GetMeetings(start, end, a.congregationID)
	if err != nil {
		return nil, err
	}

	// Criar mapa de part ID -> título da designação
	partTitles := make(map[int]string)
	for _, meeting := range meetings {
		// TGW - Tesouros da Palavra de Deus
		for _, part := range meeting.TGW {
			partTitles[part.ID] = part.Title
		}
		// FM - Reunião de Campo
		for _, part := range meeting.FM {
			partTitles[part.ID] = part.Title
		}
		// LAC - Vida Cristã
		for _, part := range meeting.LAC {
			partTitles[part.ID] = part.Title
		}
	}

	var rejeicoes []domain.Rejeicao
	timestamp := time.Now()

	for _, notif := range notifications {
		if notif.Status == "declined" {
			// Usar título real se disponível, senão fallback para nome do flag
			title := partTitles[notif.Part]
			if title == "" {
				title = getMidweekFlagName(notif.Flag)
			}

			rejeicoes = append(rejeicoes, domain.Rejeicao{
				Secao:     "Reunião Meio de Semana",
				Quem:      a.getUserName(notif.Assignee),
				OQue:      title,
				PraQuando: formatDateToBrazilian(notif.Date),
				Timestamp: timestamp,
			})
		}
	}

	return rejeicoes, nil
}

// getMidweekFlagName converts flag values to designação names
func getMidweekFlagName(flag int) string {
	switch flag {
	case 10, 11:
		return "Leitor do EBC"
	case 20:
		return "Orador/Dirigente"
	case 30:
		return "Estudante"
	case 40:
		return "Ajudante"
	case 50:
		return "Designação Especial"
	case 60:
		return "Outra Designação"
	default:
		return fmt.Sprintf("Designação (flag %d)", flag)
	}
}

// AnalyzeAllSections analyzes all sections.
func (a *APIAnalyzer) AnalyzeAllSections() ([]domain.JobResult, error) {
	sections := []string{"Partes Mecânicas", "Campo", "Testemunho Público", "Reunião Meio de Semana"}
	var results []domain.JobResult

	for _, section := range sections {
		result, _ := a.AnalyzeSection(section)
		results = append(results, *result)
	}

	return results, nil
}
