package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRejection(t *testing.T) {
	r := Rejection{
		Section:   "Field Ministry",
		Who:       "John Doe",
		What:      "Test Assignment",
		When:      "01/01/2026",
		Timestamp: time.Now(),
	}

	assert.Equal(t, "Field Ministry", r.Section)
	assert.Equal(t, "John Doe", r.Who)
	assert.Equal(t, "Test Assignment", r.What)
	assert.Equal(t, "01/01/2026", r.When)
}

func TestJobResult(t *testing.T) {
	jr := JobResult{
		Total:      5,
		Rejections: []Rejection{},
		Duration:   time.Second,
	}

	assert.Equal(t, 5, jr.Total)
	assert.Empty(t, jr.Rejections)
	assert.Equal(t, time.Second, jr.Duration)
	assert.Nil(t, jr.Error)
}

func TestJobResult_WithError(t *testing.T) {
	jr := JobResult{
		Total:      0,
		Rejections: nil,
		Duration:   0,
		Error:      assert.AnError,
	}

	assert.Equal(t, 0, jr.Total)
	assert.NotNil(t, jr.Error)
}

func TestDailyStats(t *testing.T) {
	now := time.Now()
	ds := DailyStats{
		Date:            now,
		TotalJobs:       10,
		TotalRejections: 5,
		Sections:        map[string]int{"Field Ministry": 5, "Mechanical Parts": 5},
	}

	assert.Equal(t, now, ds.Date)
	assert.Equal(t, 10, ds.TotalJobs)
	assert.Equal(t, 5, ds.TotalRejections)
	assert.Equal(t, 5, ds.Sections["Field Ministry"])
	assert.Equal(t, 5, ds.Sections["Mechanical Parts"])
}

func TestCookie(t *testing.T) {
	c := Cookie{
		Name:     "test_cookie",
		Value:    "test_value",
		Domain:   "example.com",
		Path:     "/",
		Expires:  time.Now().Add(time.Hour),
		Secure:   true,
		HttpOnly: true,
	}

	assert.Equal(t, "test_cookie", c.Name)
	assert.Equal(t, "test_value", c.Value)
	assert.Equal(t, "example.com", c.Domain)
	assert.Equal(t, "/", c.Path)
	assert.True(t, c.Secure)
	assert.True(t, c.HttpOnly)
}
