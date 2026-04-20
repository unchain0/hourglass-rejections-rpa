// Package scheduler provides job scheduling and execution functionality.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/engines/rejection_cache"
	"hourglass-rejections-rpa/src/integrations/config"
	"hourglass-rejections-rpa/src/integrations/monitoring/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	schedulerTracer          = otel.Tracer("hourglass-rejections-rpa/scheduler")
	schedulerMeter           = otel.Meter("hourglass-rejections-rpa/scheduler")
	schedulerRunCounter, _   = schedulerMeter.Int64Counter("hourglass.scheduler.runs.total")
	schedulerErrorCounter, _ = schedulerMeter.Int64Counter("hourglass.scheduler.errors.total")
	rejectionCounter, _      = schedulerMeter.Int64Counter("hourglass.rejections.total")
	analysisDuration, _      = schedulerMeter.Float64Histogram("hourglass.scheduler.analysis.duration.seconds")
)

// Analyzer defines the interface for analyzing sections.
type Analyzer interface {
	AnalyzeSection(section string) (*domain.JobResult, error)
}

// Storage defines the interface for storing rejections.
type Storage interface {
	SaveRejections(ctx context.Context, rejections []domain.Rejection) error
	RecordJobExecution(jobName string, success bool, errorMsg string) error
}

// Scheduler manages periodic analysis and notification jobs.
type Scheduler struct {
	cfg             *config.Config
	telemetryClient *telemetry.Client
	analyzer        Analyzer
	store           Storage
	cache           *cache.RejectionCache
	notifier        domain.Notifier

	runAnalysisFn func(ctx context.Context) error
}

// SetNotifier sets the notifier for sending notifications.
func (s *Scheduler) SetNotifier(n domain.Notifier) {
	s.notifier = n
}

// New creates a new Scheduler instance.
func New(cfg *config.Config, telemetryClient *telemetry.Client, analyzer Analyzer, store Storage) *Scheduler {
	return &Scheduler{
		cfg:             cfg,
		telemetryClient: telemetryClient,
		analyzer:        analyzer,
		store:           store,
		cache:           cache.New(),
	}
}

// Run starts the scheduler with the configured intervals.
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

			analysisCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			analysisCtx, span := schedulerTracer.Start(analysisCtx, "scheduled_analysis", trace.WithAttributes(
				attribute.String("scheduler.run_time", now.Format(time.RFC3339)),
			))
			err := analysisFn(analysisCtx)
			span.End()
			cancel()

			if recordErr := s.recordExecution(err); recordErr != nil {
				logger.Warn("failed to record scheduler execution", "error", recordErr)
			}

			if err != nil {
				schedulerErrorCounter.Add(ctx, 1)
				logger.Error("scheduled analysis failed", "error", err)
				s.telemetryClient.CaptureError(err, map[string]interface{}{
					"phase": "scheduled_analysis",
				})
			}

			schedulerRunCounter.Add(ctx, 1)

			nextRun = now.Add(interval)
			logger.Info("next check scheduled", "at", nextRun.Format("15:04"))
		}
	}
}

func (s *Scheduler) recordExecution(runErr error) error {
	if s.store == nil {
		return nil
	}

	errorMsg := ""
	if runErr != nil {
		errorMsg = runErr.Error()
	}

	return s.store.RecordJobExecution("scheduled_analysis", runErr == nil, errorMsg)
}

func (s *Scheduler) calculateInterval(now time.Time) time.Duration {
	hour := now.Hour()
	interval := intervalForHour(hour)

	if hour >= 6 && hour < 22 {
		slog.Info("business hours check", "hour", hour, "next_interval", interval)
	} else {
		slog.Info("night hours check", "hour", hour, "next_interval", interval)
	}

	return interval
}

func (s *Scheduler) runAnalysis(ctx context.Context) error {
	start := time.Now()
	ctx, span := schedulerTracer.Start(ctx, "run_analysis")
	defer span.End()
	defer func() {
		analysisDuration.Record(ctx, time.Since(start).Seconds())
	}()
	var allRejections []domain.Rejection
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, section := range domain.AllSections {
		wg.Add(1)
		go func(sec string) {
			defer wg.Done()
			sectionCtx, sectionSpan := schedulerTracer.Start(ctx, "analyze_section", trace.WithAttributes(attribute.String("section", sec)))
			defer sectionSpan.End()

			slog.Info("analyzing section", "section", sec)

			result, err := s.analyzer.AnalyzeSection(sec)
			if err != nil {
				sectionSpan.RecordError(err)
				slog.Error("failed to analyze section", "section", sec, "error", err)
				s.telemetryClient.CaptureError(err, map[string]interface{}{
					"section": sec,
					"phase":   "analysis",
				})
				return
			}

			if result.Error != nil {
				sectionSpan.RecordError(result.Error)
				slog.Error("analysis returned error", "section", sec, "error", result.Error)
				s.telemetryClient.CaptureError(result.Error, map[string]interface{}{
					"section": sec,
					"phase":   "analysis_result",
					"total":   result.Total,
				})
				return
			}

			slog.Info("section analysis complete", "section", sec, "total", result.Total)

			if len(result.Rejections) > 0 {
				rejectionCounter.Add(sectionCtx, int64(len(result.Rejections)), metric.WithAttributes(attribute.String("section", sec)))
				mu.Lock()
				allRejections = append(allRejections, result.Rejections...)
				mu.Unlock()

				if err := s.store.SaveRejections(ctx, result.Rejections); err != nil {
					slog.Error("failed to save rejections", "section", sec, "error", err)
					s.telemetryClient.CaptureError(err, map[string]interface{}{
						"section": sec,
						"phase":   "save_rejections",
						"count":   len(result.Rejections),
					})
				}
			}
		}(section)
	}

	wg.Wait()
	return s.sendNotifications(allRejections, time.Since(start))
}

func (s *Scheduler) sendNotifications(rejections []domain.Rejection, duration time.Duration) error {
	if len(rejections) == 0 {
		return nil
	}

	if !s.cache.HasChanges(rejections) {
		slog.Info("skipping notification - no changes from last check")
		return nil
	}

	if s.notifier == nil {
		slog.Warn("no notifier configured, skipping notification")
		return nil
	}

	summary := buildNotificationSummary(rejections)

	if err := s.notifier.SendJobCompletion(summary, duration); err != nil {
		slog.Error("failed to send notification", "error", err)
		s.telemetryClient.CaptureError(err, map[string]interface{}{
			"phase": "send_notification",
		})
		return err
	}

	slog.Info("notification sent", "summary", summary, "duration", duration)
	return nil
}
