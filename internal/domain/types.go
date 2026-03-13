// Package domain contains the core business logic and data structures.
package domain

import (
	"time"
)

// Rejection represents a rejection record.
type Rejection struct {
	Section   string    `json:"section"`
	Who       string    `json:"who"`
	What      string    `json:"what"`
	When      string    `json:"when"`
	Timestamp time.Time `json:"timestamp"`
}

// JobResult represents the result of a scraping job for a specific section.
type JobResult struct {
	Section    string
	Total      int
	Rejections []Rejection
	Duration   time.Duration
	Error      error
}

// DailyStats represents the statistics for a day's scraping jobs.
type DailyStats struct {
	Date            time.Time
	TotalJobs       int
	TotalRejections int
	Sections        map[string]int
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
