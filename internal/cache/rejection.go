package cache

import (
	"log/slog"
	"sync"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

type RejectionCache struct {
	mu         sync.RWMutex
	lastResult []domain.Rejeicao
	lastCheck  time.Time
}

func New() *RejectionCache {
	return &RejectionCache{}
}

func (c *RejectionCache) HasChanges(newRejections []domain.Rejeicao) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.lastResult) == 0 && len(newRejections) > 0 {
		c.lastResult = newRejections
		c.lastCheck = time.Now()
		return true
	}

	if len(newRejections) == 0 && len(c.lastResult) == 0 {
		c.lastCheck = time.Now()
		return false
	}

	if len(newRejections) != len(c.lastResult) {
		c.lastResult = newRejections
		c.lastCheck = time.Now()
		return true
	}

	for i, new := range newRejections {
		if i >= len(c.lastResult) {
			break
		}
		old := c.lastResult[i]
		if new.Secao != old.Secao || new.Quem != old.Quem || new.OQue != old.OQue {
			c.lastResult = newRejections
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

func (c *RejectionCache) LastCheck() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCheck
}
