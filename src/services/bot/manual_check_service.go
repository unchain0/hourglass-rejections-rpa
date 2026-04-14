package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/i18n"
	"hourglass-rejections-rpa/src/integrations/monitoring/sentry"
)

type noRejectionsSender func(chatID int64, message string) error
type rejectionsSender func(chatID int64, rejections []domain.Rejection) error

type manualCheckService struct {
	analyzer         Analyzer
	sentryClient     *sentry.Client
	prefManager      *preferences.PreferenceManager
	sendNoRejections noRejectionsSender
	sendRejections   rejectionsSender
}

func newManualCheckService(
	analyzer Analyzer,
	sentryClient *sentry.Client,
	prefManager *preferences.PreferenceManager,
	sendNoRejections noRejectionsSender,
	sendRejections rejectionsSender,
) *manualCheckService {
	return &manualCheckService{
		analyzer:         analyzer,
		sentryClient:     sentryClient,
		prefManager:      prefManager,
		sendNoRejections: sendNoRejections,
		sendRejections:   sendRejections,
	}
}

func (s *manualCheckService) run(ctx context.Context, targetChatID int64) error {
	logger := slog.Default()
	start := time.Now()

	pref, err := s.prefManager.Get(targetChatID)
	if err != nil {
		logger.Error("failed to get user preferences", "chat_id", targetChatID, "error", err)
		if s.sentryClient != nil {
			s.sentryClient.CaptureError(err, map[string]interface{}{
				"phase":   "get_user_preferences",
				"chat_id": targetChatID,
			})
		}
		return fmt.Errorf("failed to get user preferences: %w", err)
	}

	if pref == nil {
		logger.Error("user preferences not found", "chat_id", targetChatID)
		if s.sentryClient != nil {
			s.sentryClient.CaptureMessage("user preferences not found", "error")
		}
		return fmt.Errorf("user preferences not found")
	}

	lang := s.prefManager.GetLanguage(targetChatID)
	userSections := pref.Sections()
	logger.Info("user preferences loaded", "chat_id", targetChatID, "sections", userSections, "sections_count", len(userSections))

	if len(userSections) == 0 {
		logger.Info("no sections configured, sending message", "chat_id", targetChatID)
		return s.sendNoRejections(targetChatID, i18n.Localize(lang, "no_sections_selected", nil))
	}

	allRejections, err := s.collectRejections(ctx, targetChatID, userSections)
	if err != nil {
		return err
	}

	logger.Info("analysis complete", "chat_id", targetChatID, "total_rejections", len(allRejections), "duration", time.Since(start))
	if len(allRejections) == 0 {
		logger.Info("no rejections found, sending message", "chat_id", targetChatID)
		return s.sendNoRejections(targetChatID, i18n.Localize(lang, "no_rejections_found", nil))
	}

	logger.Info("sending rejections notification", "chat_id", targetChatID, "count", len(allRejections))
	return s.sendRejections(targetChatID, allRejections)
}

func (s *manualCheckService) collectRejections(ctx context.Context, targetChatID int64, sections []string) ([]domain.Rejection, error) {
	logger := slog.Default()
	var allRejections []domain.Rejection

	for _, section := range sections {
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, stopping analysis", "chat_id", targetChatID)
			return nil, ctx.Err()
		default:
		}

		sectionStart := time.Now()
		logger.Info("analyzing section for user", "section", section, "chat_id", targetChatID)

		result, err := s.analyzer.AnalyzeSection(section)
		if err != nil {
			logger.Error("failed to analyze section", "section", section, "error", err, "duration", time.Since(sectionStart))
			s.captureAnalysisError(err, targetChatID, section, sectionStart, "analyze_section_for_user", nil)
			continue
		}

		if result.Error != nil {
			logger.Error("analysis returned error", "section", section, "error", result.Error, "duration", time.Since(sectionStart))
			s.captureAnalysisError(result.Error, targetChatID, section, sectionStart, "analysis_result_for_user", result)
			continue
		}

		logger.Info("section analyzed", "section", section, "rejections_count", len(result.Rejections), "duration", time.Since(sectionStart))
		allRejections = append(allRejections, result.Rejections...)
	}

	return allRejections, nil
}

func (s *manualCheckService) captureAnalysisError(err error, chatID int64, section string, sectionStart time.Time, phase string, result *domain.JobResult) {
	if s.sentryClient == nil {
		return
	}

	extras := map[string]interface{}{
		"section":  section,
		"phase":    phase,
		"chat_id":  chatID,
		"duration": time.Since(sectionStart).String(),
	}
	if result != nil {
		extras["total"] = result.Total
	}

	s.sentryClient.CaptureError(err, extras)
}
