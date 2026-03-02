package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRejeicao(t *testing.T) {
	r := Rejeicao{
		Secao:     "Campo",
		Quem:      "John Doe",
		OQue:      "Test Assignment",
		PraQuando: "01/01/2026",
		Timestamp: time.Now(),
	}

	assert.Equal(t, "Campo", r.Secao)
	assert.Equal(t, "John Doe", r.Quem)
	assert.Equal(t, "Test Assignment", r.OQue)
	assert.Equal(t, "01/01/2026", r.PraQuando)
}

func TestJobResult(t *testing.T) {
	jr := JobResult{
		Total:     5,
		Rejeicoes: []Rejeicao{},
		Duration:  time.Second,
	}

	assert.Equal(t, 5, jr.Total)
	assert.Empty(t, jr.Rejeicoes)
	assert.Equal(t, time.Second, jr.Duration)
	assert.Nil(t, jr.Error)
}

func TestJobResult_WithError(t *testing.T) {
	jr := JobResult{
		Total:     0,
		Rejeicoes: nil,
		Duration:  0,
		Error:     assert.AnError,
	}

	assert.Equal(t, 0, jr.Total)
	assert.NotNil(t, jr.Error)
}

func TestDailyStats(t *testing.T) {
	now := time.Now()
	ds := DailyStats{
		Date:      now,
		TotalJobs: 10,
		TotalRej:  5,
		Sections:  map[string]int{"Campo": 5, "Partes Mecânicas": 5},
	}

	assert.Equal(t, now, ds.Date)
	assert.Equal(t, 10, ds.TotalJobs)
	assert.Equal(t, 5, ds.TotalRej)
	assert.Equal(t, 5, ds.Sections["Campo"])
	assert.Equal(t, 5, ds.Sections["Partes Mecânicas"])
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
