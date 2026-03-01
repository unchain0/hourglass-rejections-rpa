// Package domain contains the core business logic and data structures.
package domain

import (
	"time"
)

// Rejeicao represents a rejection record.
type Rejeicao struct {
	Secao     string    `json:"secao"`
	Quem      string    `json:"quem"`
	OQue      string    `json:"oque"`
	PraQuando string    `json:"pra_quando"`
	Timestamp time.Time `json:"timestamp"`
}

// JobResult represents the result of a scraping job for a specific section.
type JobResult struct {
	Secao     string
	Total     int
	Rejeicoes []Rejeicao
	Duration  time.Duration
	Error     error
}

// DailyStats represents the statistics for a day's scraping jobs.
type DailyStats struct {
	Date      time.Time
	TotalJobs int
	TotalRej  int
	Sections  map[string]int
}

// Cookie represents a browser cookie for persistence.
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires"`
	Secure   bool      `json:"secure"`
	HttpOnly bool      `json:"http_only"`
}
