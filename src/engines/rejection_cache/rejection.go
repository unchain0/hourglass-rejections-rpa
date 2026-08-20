// Package cache provides in-memory caching utilities for rejection checks.
package cache

import (
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
)

// RejectionCache stores the latest rejection snapshot and comparison time.
type RejectionCache struct {
	mu         sync.RWMutex
	lastResult []domain.Rejection
	lastCheck  time.Time
}

// New creates a new RejectionCache.
func New() *RejectionCache {
	return &RejectionCache{}
}

// HasChanges reports whether the provided rejections differ from the cached result.
func (c *RejectionCache) HasChanges(newRejections []domain.Rejection) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	newRejections = canonicalize(newRejections)

	if len(c.lastResult) == 0 && len(newRejections) > 0 {
		c.lastResult = slices.Clone(newRejections)
		c.lastCheck = time.Now()
		return true
	}

	if len(newRejections) == 0 && len(c.lastResult) == 0 {
		c.lastCheck = time.Now()
		return false
	}

	if len(newRejections) != len(c.lastResult) {
		c.lastResult = slices.Clone(newRejections)
		c.lastCheck = time.Now()
		return true
	}

	for i, new := range newRejections {
		old := c.lastResult[i]
		if new.Section != old.Section || new.Who != old.Who || new.What != old.What {
			c.lastResult = slices.Clone(newRejections)
			c.lastCheck = time.Now()
			return true
		}
	}

	c.lastCheck = time.Now()
	slog.Info("no changes detected since last check, skipping notification",
		"last_check", c.lastCheck,
		"rejections_count", len(newRejections))
	return false
}

// LastCheck returns the timestamp of the most recent cache comparison.
func (c *RejectionCache) LastCheck() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCheck
}

func (c *RejectionCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastResult = nil
}

func canonicalize(rejections []domain.Rejection) []domain.Rejection {
	canonical := slices.Clone(rejections)
	slices.SortFunc(canonical, func(left, right domain.Rejection) int {
		if left.Section != right.Section {
			return strings.Compare(left.Section, right.Section)
		}
		if left.Who != right.Who {
			return strings.Compare(left.Who, right.Who)
		}
		return strings.Compare(left.What, right.What)
	})
	return canonical
}
