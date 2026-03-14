package api

import (
	"fmt"
	"sync"
	"time"

	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/i18n"
)

// APIAnalyzer uses the Hourglass REST API to detect rejections.
type APIAnalyzer struct {
	client          *Client
	userCache       map[int]*User
	userCacheOnce   sync.Once
	userCacheErr    error
	congregationID  int
	daysToLookAhead int
	language        string
}

// NewAPIAnalyzer creates a new API-based analyzer.
func NewAPIAnalyzer(client *Client) *APIAnalyzer {
	return &APIAnalyzer{
		client:          client,
		userCache:       make(map[int]*User),
		congregationID:  48092,
		daysToLookAhead: 730,
		language:        "en",
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

func (a *APIAnalyzer) SetLanguage(lang string) {
	a.language = lang
}

// AnalyzeSection analyzes a specific section for rejections.
// Supported sections: "Mechanical Parts", "Field Ministry", "Public Witnessing", "Midweek Meeting"
func (a *APIAnalyzer) AnalyzeSection(section string) (*domain.JobResult, error) {
	start := time.Now()

	a.userCacheOnce.Do(func() {
		a.userCacheErr = a.loadUsers()
	})
	if a.userCacheErr != nil {
		return &domain.JobResult{
			Section:  section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("failed to load users: %w", a.userCacheErr),
		}, nil
	}

	var rejections []domain.Rejection
	var err error

	switch section {
	case "Mechanical Parts", "avattendant", "Partes Mecânicas":
		rejections, err = a.analyzeMechanicalParts()
	case "Field Ministry", "fsMeeting", "Campo":
		rejections, err = a.analyzeFieldMinistry()
	case "Public Witnessing", "publicWitnessing", "Testemunho Público":
		rejections, err = a.analyzePublicWitnessing()
	case "Midweek Meeting", "midweekMeeting", "Reunião Meio de Semana":
		rejections, err = a.analyzeMidweekMeetings()
	default:
		return &domain.JobResult{
			Section:  section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("unknown section: %s", section),
		}, nil
	}

	if err != nil {
		return &domain.JobResult{
			Section:  section,
			Duration: time.Since(start),
			Error:    fmt.Errorf("failed to analyze section %s: %w", section, err),
		}, nil
	}

	return &domain.JobResult{
		Section:    section,
		Total:      len(rejections),
		Rejections: rejections,
		Duration:   time.Since(start),
		Error:      nil,
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
func (a *APIAnalyzer) analyzeGenericNotifications(sectionName, notificationType string) ([]domain.Rejection, error) {
	now := time.Now()
	start := now.Format("2006-01-02")
	end := now.AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	notifications, err := a.client.GetNotifications(start, end, notificationType)
	if err != nil {
		return nil, err
	}

	var rejections []domain.Rejection
	seen := make(map[string]bool)
	timestamp := now

	for _, notif := range notifications {
		if notif.Status == "declined" {
			rejection := domain.Rejection{
				Section:   sectionName,
				Who:       a.getUserName(notif.Assignee),
				What:      getFriendlyTypeName(notif.Type),
				When:      i18n.FormatDate(notif.Date, a.language),
				Timestamp: timestamp,
			}
			key := fmt.Sprintf("%s|%s|%s|%s", rejection.Section, rejection.Who, rejection.What, rejection.When)
			if !seen[key] {
				seen[key] = true
				rejections = append(rejections, rejection)
			}
		}
	}

	return rejections, nil
}

// analyzeMechanicalParts analyzes mechanical assignments for rejections.
func (a *APIAnalyzer) analyzeMechanicalParts() ([]domain.Rejection, error) {
	return a.analyzeGenericNotifications("Mechanical Parts", "ava")
}

// getFriendlyTypeName converts technical type names to user-friendly names
func getFriendlyTypeName(typeName string) string {
	switch typeName {
	case "ava":
		return "Audio/Video & Indicators"
	case "video":
		return "Video"
	case "console":
		return "Console"
	case "mics":
		return "Microphone"
	case "attendant":
		return "Attendant"
	case "pubwit":
		return "Public Witnessing"
	case "fm":
		return "Field Ministry Meeting"
	default:
		return typeName
	}
}

func (a *APIAnalyzer) analyzeFieldMinistry() ([]domain.Rejection, error) {
	return a.analyzeGenericNotifications("Field Ministry", "fm")
}

func (a *APIAnalyzer) analyzePublicWitnessing() ([]domain.Rejection, error) {
	return a.analyzeGenericNotifications("Public Witnessing", "pubwit")
}

// analyzeMidweekMeetings analyzes midweek meeting assignments for rejections.
func (a *APIAnalyzer) analyzeMidweekMeetings() ([]domain.Rejection, error) {
	start := time.Now().Format("2006-01-02")
	end := time.Now().AddDate(0, 0, a.daysToLookAhead).Format("2006-01-02")

	notifications, err := a.client.GetNotifications(start, end, "mm")
	if err != nil {
		return nil, err
	}

	meetings, err := a.client.GetMeetings(start, end, a.congregationID)
	if err != nil {
		return nil, err
	}

	partTitles := make(map[int]string)
	for _, meeting := range meetings {
		for _, part := range meeting.TGW {
			partTitles[part.ID] = part.Title
		}
		for _, part := range meeting.FM {
			partTitles[part.ID] = part.Title
		}
		for _, part := range meeting.LAC {
			partTitles[part.ID] = part.Title
		}
	}

	var rejections []domain.Rejection
	seen := make(map[string]bool)
	timestamp := time.Now()

	for _, notif := range notifications {
		if notif.Status == "declined" {
			title := partTitles[notif.Part]
			if title == "" {
				title = getMidweekFlagName(notif.Flag)
			}

			rejection := domain.Rejection{
				Section:   "Midweek Meeting",
				Who:       a.getUserName(notif.Assignee),
				What:      title,
				When:      i18n.FormatDate(notif.Date, a.language),
				Timestamp: timestamp,
			}

			key := fmt.Sprintf("%s|%s|%s|%s", rejection.Section, rejection.Who, rejection.What, rejection.When)
			if !seen[key] {
				seen[key] = true
				rejections = append(rejections, rejection)
			}
		}
	}

	return rejections, nil
}

// getMidweekFlagName converts flag values to assignment names
func getMidweekFlagName(flag int) string {
	switch flag {
	case 10, 11:
		return "Bible Reading"
	case 20:
		return "Speaker/Chairman"
	case 30:
		return "Student"
	case 40:
		return "Assistant"
	case 50:
		return "Special Assignment"
	case 60:
		return "Other Assignment"
	default:
		return fmt.Sprintf("Assignment (flag %d)", flag)
	}
}

// AnalyzeAllSections analyzes all sections.
func (a *APIAnalyzer) AnalyzeAllSections() ([]domain.JobResult, error) {
	var results []domain.JobResult

	for _, section := range domain.AllSections {
		result, _ := a.AnalyzeSection(section)
		results = append(results, *result)
	}

	return results, nil
}
