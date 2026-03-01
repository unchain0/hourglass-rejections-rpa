// Package domain contains the core business logic and data structures.
package domain

import (
	"context"
	"time"
)

// Scraper defines the contract for browser automation.
type Scraper interface {
	Setup(ctx context.Context) error
	Login(ctx context.Context) error
	IsAuthenticated(ctx context.Context) (bool, error)
	AnalyzeSection(ctx context.Context, secao string) (*JobResult, error)
	Close() error
}

// Storage defines the contract for persistence.
type Storage interface {
	Save(ctx context.Context, rejeicoes []Rejeicao) error
	LoadCookies() ([]Cookie, error)
	SaveCookies(cookies []Cookie) error
}

// Notifier defines the contract for notifications.
type Notifier interface {
	SendJobCompletion(summary string, duration time.Duration) error
	SendJobFailure(step string, err error) error
	SendDailyReport(stats DailyStats) error
}
