package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	if s == nil {
		t.Fatal("New returned nil")
	}

	if s.cron == nil {
		t.Error("cron not initialized")
	}

	if s.logger == nil {
		t.Error("logger not set")
	}

	if s.jobs == nil {
		t.Error("jobs map not initialized")
	}
}

func TestScheduler_AddJob(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	job := func(ctx context.Context) error {
		return nil
	}

	err := s.AddJob("test-job", "0 0 * * * *", job)
	if err != nil {
		t.Errorf("AddJob() error = %v", err)
	}

	if _, exists := s.jobs["test-job"]; !exists {
		t.Error("job not added to jobs map")
	}
}

func TestScheduler_AddDailyJob(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	job := func(ctx context.Context) error {
		return nil
	}

	err := s.AddDailyJob("morning-job", 9, 0, job)
	if err != nil {
		t.Errorf("AddDailyJob() error = %v", err)
	}

	_, exists := s.NextRun("morning-job")
	if !exists {
		t.Error("job not found after scheduling")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	s.Start()
	time.Sleep(10 * time.Millisecond)
	s.Stop()
}

func TestScheduler_ListJobs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	job := func(ctx context.Context) error {
		return nil
	}

	s.AddJob("job1", "0 0 * * * *", job)
	s.AddJob("job2", "0 0 * * * *", job)
	s.AddJob("job3", "0 0 * * * *", job)

	jobs := s.ListJobs()
	if len(jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(jobs))
	}

	jobMap := make(map[string]bool)
	for _, name := range jobs {
		jobMap[name] = true
	}

	if !jobMap["job1"] || !jobMap["job2"] || !jobMap["job3"] {
		t.Error("not all jobs were listed")
	}
}

func TestScheduler_NextRun_NotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	_, exists := s.NextRun("non-existent-job")
	if exists {
		t.Error("expected false for non-existent job")
	}
}

func TestScheduler_NextRun_Found(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	job := func(ctx context.Context) error {
		return nil
	}

	// Start scheduler so entry.Next is populated
	s.AddJob("test-job", "* * * * * *", job)
	s.Start()

	nextTime, exists := s.NextRun("test-job")
	s.Stop()

	if !exists {
		t.Error("expected true for existing job")
	}

	if nextTime.IsZero() {
		t.Error("expected non-zero next run time")
	}
}

func TestScheduler_JobExecution_Error(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	jobExecuted := make(chan bool, 1)
	job := func(ctx context.Context) error {
		jobExecuted <- true
		return fmt.Errorf("test error")
	}

	s.AddJob("error-job", "* * * * * *", job)
	s.Start()

	// Wait for job to execute (should run immediately with * * * * * *)
	select {
	case <-jobExecuted:
		// Job executed successfully
	case <-time.After(2 * time.Second):
		t.Error("job was not executed within timeout")
	}

	s.Stop()
}

func TestScheduler_JobExecution_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	jobExecuted := make(chan bool, 1)
	job := func(ctx context.Context) error {
		jobExecuted <- true
		return nil
	}

	s.AddJob("success-job", "* * * * * *", job)
	s.Start()

	// Wait for job to execute (should run immediately with * * * * * *)
	select {
	case <-jobExecuted:
		// Job executed successfully
	case <-time.After(2 * time.Second):
		t.Error("job was not executed within timeout")
	}

	s.Stop()
}

func TestScheduler_AddJob_InvalidSchedule(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := New(logger)

	job := func(ctx context.Context) error {
		return nil
	}

	err := s.AddJob("invalid-job", "invalid-cron", job)
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}
