package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// JobFunc is the function signature for scheduled jobs.
type JobFunc func(ctx context.Context) error

// Scheduler handles cron-based job scheduling.
type Scheduler struct {
	cron   *cron.Cron
	logger *slog.Logger
	jobs   map[string]cron.EntryID
}

// New creates a new Scheduler instance.
func New(logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:   cron.New(cron.WithSeconds()),
		logger: logger,
		jobs:   make(map[string]cron.EntryID),
	}
}

// AddJob schedules a job to run according to the given cron expression.
func (s *Scheduler) AddJob(name string, schedule string, job JobFunc) error {
	entryID, err := s.cron.AddFunc(schedule, func() {
		s.logger.Info("starting scheduled job", "job", name, "time", time.Now())
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		
		if err := job(ctx); err != nil {
			s.logger.Error("job failed", "job", name, "error", err)
		} else {
			s.logger.Info("job completed successfully", "job", name)
		}
	})
	
	if err != nil {
		return fmt.Errorf("failed to add job %s: %w", name, err)
	}
	
	s.jobs[name] = entryID
	s.logger.Info("job scheduled", "job", name, "schedule", schedule)
	return nil
}

// AddDailyJob schedules a job to run daily at a specific hour and minute.
func (s *Scheduler) AddDailyJob(name string, hour, minute int, job JobFunc) error {
	// Cron expression: second minute hour day month dayOfWeek
	// 0 <minute> <hour> * * * means daily at hour:minute:00
	schedule := fmt.Sprintf("0 %d %d * * *", minute, hour)
	return s.AddJob(name, schedule, job)
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info("scheduler started")
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {	s.cron.Stop()
	s.logger.Info("scheduler stopped")
}

// NextRun returns the next scheduled run time for a job.
func (s *Scheduler) NextRun(name string) (time.Time, bool) {
	entryID, exists := s.jobs[name]
	if !exists {
		return time.Time{}, false
	}
	
	entry := s.cron.Entry(entryID)
	return entry.Next, true
}

// ListJobs returns a list of all scheduled job names.
func (s *Scheduler) ListJobs() []string {
	names := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		names = append(names, name)
	}
	return names
}
