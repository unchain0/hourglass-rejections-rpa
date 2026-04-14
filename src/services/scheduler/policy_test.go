package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"hourglass-rejections-rpa/src/domain_models"
)

func TestIntervalForHour(t *testing.T) {
	assert.Equal(t, 30*time.Minute, intervalForHour(6))
	assert.Equal(t, 30*time.Minute, intervalForHour(21))
	assert.Equal(t, 2*time.Hour, intervalForHour(5))
	assert.Equal(t, 2*time.Hour, intervalForHour(22))
}

func TestBuildNotificationSummary(t *testing.T) {
	t.Run("empty rejections", func(t *testing.T) {
		assert.Equal(t, "0 rejections detected", buildNotificationSummary(nil))
	})

	t.Run("preserves first seen section order", func(t *testing.T) {
		rejections := []domain.Rejection{
			{Section: "Field Ministry"},
			{Section: "Mechanical Parts"},
			{Section: "Field Ministry"},
		}

		assert.Equal(t, "3 rejections detected. Sections: Field Ministry (2), Mechanical Parts (1)", buildNotificationSummary(rejections))
	})
}
