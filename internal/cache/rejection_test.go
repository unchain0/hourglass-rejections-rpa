package cache

import (
	"testing"
	"time"

	"hourglass-rejections-rpa/internal/domain"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.lastResult != nil {
		t.Error("new cache should have nil lastResult")
	}
	if !c.lastCheck.IsZero() {
		t.Error("new cache should have zero lastCheck")
	}
}

func TestRejectionCache_HasChanges_FirstCheckWithRejections(t *testing.T) {
	c := New()
	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	if !c.HasChanges(rejections) {
		t.Error("first check with rejections should return true")
	}

	if len(c.lastResult) != 1 {
		t.Errorf("expected 1 rejection in cache, got %d", len(c.lastResult))
	}

	if c.lastCheck.IsZero() {
		t.Error("lastCheck should be set after first check")
	}
}

func TestRejectionCache_HasChanges_FirstCheckEmpty(t *testing.T) {
	c := New()
	rejections := []domain.Rejeicao{}

	if c.HasChanges(rejections) {
		t.Error("first check with no rejections should return false")
	}

	if c.lastCheck.IsZero() {
		t.Error("lastCheck should be set even with empty results")
	}
}

func TestRejectionCache_HasChanges_SameResults(t *testing.T) {
	c := New()
	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	c.HasChanges(rejections)

	if c.HasChanges(rejections) {
		t.Error("same results should return false")
	}
}

func TestRejectionCache_HasChanges_DifferentCount(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	rejections2 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
		{Secao: "Campo", Quem: "Jane", OQue: "Test2", PraQuando: "02/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different count should return true")
	}

	if len(c.lastResult) != 2 {
		t.Errorf("expected 2 rejections in cache, got %d", len(c.lastResult))
	}
}

func TestRejectionCache_HasChanges_DifferentSecao(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	rejections2 := []domain.Rejeicao{
		{Secao: "Partes Mecânicas", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different Secao should return true")
	}
}

func TestRejectionCache_HasChanges_DifferentQuem(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	rejections2 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Jane", OQue: "Test", PraQuando: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different Quem should return true")
	}
}

func TestRejectionCache_HasChanges_DifferentOQue(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	rejections2 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Different", PraQuando: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different OQue should return true")
	}
}

func TestRejectionCache_HasChanges_SamePraQuandoDifferent(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	rejections2 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "02/03/2026"},
	}

	c.HasChanges(rejections1)

	if c.HasChanges(rejections2) {
		t.Error("PraQuando is not compared, so should return false")
	}
}

func TestRejectionCache_HasChanges_EmptyToNonEmpty(t *testing.T) {
	c := New()
	c.HasChanges([]domain.Rejeicao{})

	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}

	if !c.HasChanges(rejections) {
		t.Error("empty to non-empty should return true")
	}
}

func TestRejectionCache_HasChanges_NonEmptyToEmpty(t *testing.T) {
	c := New()
	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	c.HasChanges(rejections)

	if !c.HasChanges([]domain.Rejeicao{}) {
		t.Error("non-empty to empty should return true (different count)")
	}
}

func TestRejectionCache_LastCheck(t *testing.T) {
	c := New()

	if !c.LastCheck().IsZero() {
		t.Error("LastCheck should be zero for new cache")
	}

	c.HasChanges([]domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	})

	if c.LastCheck().IsZero() {
		t.Error("LastCheck should be set after HasChanges")
	}
}

func TestRejectionCache_ConcurrentAccess(t *testing.T) {
	c := New()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			c.HasChanges([]domain.Rejeicao{
				{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
			})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			c.LastCheck()
		}
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent access test timed out")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent access test timed out")
	}
}

func TestRejectionCache_MultipleRejectionsOrder(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test1", PraQuando: "01/03/2026"},
		{Secao: "Campo", Quem: "Jane", OQue: "Test2", PraQuando: "02/03/2026"},
	}
	rejections2 := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Jane", OQue: "Test2", PraQuando: "02/03/2026"},
		{Secao: "Campo", Quem: "John", OQue: "Test1", PraQuando: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different order should be detected as changes (index-based comparison)")
	}
}
