package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"hourglass-rejections-rpa/internal/cache"
	"hourglass-rejections-rpa/internal/config"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/sentry"
)

type Analyzer interface {
	AnalyzeSection(section string) (*domain.JobResult, error)
}

type Storage interface {
	Save(ctx context.Context, rejections []domain.Rejeicao) error
}

type Scheduler struct {
	cfg          *config.Config
	sentryClient *sentry.Client
	analyzer     Analyzer
	store        Storage
	cache        *cache.RejectionCache

	runAnalysisFn func(ctx context.Context) error
}

func New(cfg *config.Config, sentryClient *sentry.Client, analyzer Analyzer, store Storage) *Scheduler {
	return &Scheduler{
		cfg:          cfg,
		sentryClient: sentryClient,
		analyzer:     analyzer,
		store:        store,
		cache:        cache.New(),
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	logger := slog.Default()
	logger.Info("starting smart scheduler", "business_hours", "30min", "night_hours", "2h")

	return s.runWithTicker(ctx, time.NewTicker(1*time.Minute))
}

func (s *Scheduler) runWithTicker(ctx context.Context, ticker *time.Ticker) error {
	logger := slog.Default()
	defer ticker.Stop()

	nextRun := time.Now()

	for {
		select {
		case <-ctx.Done():
			logger.Info("scheduler stopped")
			return nil
		case now := <-ticker.C:
			if now.Before(nextRun) {
				continue
			}

			interval := s.calculateInterval(now)

			logger.Info("running scheduled analysis", "time", now.Format("15:04"))

			analysisFn := s.runAnalysis
			if s.runAnalysisFn != nil {
				analysisFn = s.runAnalysisFn
			}
			if err := analysisFn(ctx); err != nil {
				logger.Error("scheduled analysis failed", "error", err)
				s.sentryClient.CaptureError(err, map[string]interface{}{
					"phase": "scheduled_analysis",
				})
			}

			nextRun = now.Add(interval)
			logger.Info("next check scheduled", "at", nextRun.Format("15:04"))
		}
	}
}

func (s *Scheduler) calculateInterval(now time.Time) time.Duration {
	hour := now.Hour()
	var interval time.Duration

	if hour >= 6 && hour < 22 {
		interval = 30 * time.Minute
		slog.Info("business hours check", "hour", hour, "next_interval", interval)
	} else {
		interval = 2 * time.Hour
		slog.Info("night hours check", "hour", hour, "next_interval", interval)
	}

	return interval
}

func (s *Scheduler) runAnalysis(ctx context.Context) error {
	var allRejections []domain.Rejeicao
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, section := range domain.AllSections {
		wg.Add(1)
		go func(sec string) {
			defer wg.Done()

			slog.Info("analyzing section", "section", sec)

			result, err := s.analyzer.AnalyzeSection(sec)
			if err != nil {
				slog.Error("failed to analyze section", "section", sec, "error", err)
				s.sentryClient.CaptureError(err, map[string]interface{}{
					"section": sec,
					"phase":   "analysis",
				})
				return
			}

			if result.Error != nil {
				slog.Error("analysis returned error", "section", sec, "error", result.Error)
				s.sentryClient.CaptureError(result.Error, map[string]interface{}{
					"section": sec,
					"phase":   "analysis_result",
					"total":   result.Total,
				})
				return
			}

			slog.Info("section analysis complete", "section", sec, "total", result.Total)

			if len(result.Rejeicoes) > 0 {
				mu.Lock()
				allRejections = append(allRejections, result.Rejeicoes...)
				mu.Unlock()

				if err := s.store.Save(ctx, result.Rejeicoes); err != nil {
					slog.Error("failed to save rejections", "section", sec, "error", err)
					s.sentryClient.CaptureError(err, map[string]interface{}{
						"section": sec,
						"phase":   "save_rejections",
						"count":   len(result.Rejeicoes),
					})
				}
			}
		}(section)
	}

	wg.Wait()
	return s.sendNotifications(allRejections)
}

func (s *Scheduler) sendNotifications(rejections []domain.Rejeicao) error {
	if len(rejections) == 0 {
		return nil
	}

	if !s.cache.HasChanges(rejections) {
		slog.Info("skipping notification - no changes from last check")
		return nil
	}

	return nil
}
