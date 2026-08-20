package cache

import (
	"testing"
	"time"

	"hourglass-rejections-rpa/src/domain_models"
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
	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
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
	rejections := []domain.Rejection{}

	if c.HasChanges(rejections) {
		t.Error("first check with no rejections should return false")
	}

	if c.lastCheck.IsZero() {
		t.Error("lastCheck should be set even with empty results")
	}
}

func TestRejectionCache_HasChanges_FirstCheckNilSlice(t *testing.T) {
	c := New()

	if c.HasChanges(nil) {
		t.Error("first check with nil rejections should return false")
	}

	if c.lastCheck.IsZero() {
		t.Error("lastCheck should be set for nil input")
	}
}

func TestRejectionCache_HasChanges_SameResults(t *testing.T) {
	c := New()
	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}

	c.HasChanges(rejections)

	if c.HasChanges(rejections) {
		t.Error("same results should return false")
	}
}

func TestRejectionCache_HasChanges_IgnoresCompletionOrder(t *testing.T) {
	c := New()
	first := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test"},
		{Section: "Public Witnessing", Who: "Jane", What: "Test"},
	}
	second := []domain.Rejection{first[1], first[0]}

	requireFirst := c.HasChanges(first)
	if !requireFirst {
		t.Fatal("first snapshot should be a change")
	}
	if c.HasChanges(second) {
		t.Fatal("reordered equivalent results should not be treated as changes")
	}
}
func TestRejectionCache_HasChanges_DifferentCount(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	rejections2 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
		{Section: "Field Ministry", Who: "Jane", What: "Test2", When: "02/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different count should return true")
	}

	if len(c.lastResult) != 2 {
		t.Errorf("expected 2 rejections in cache, got %d", len(c.lastResult))
	}
}

func TestCanonicalize_SortsStableFields(t *testing.T) {
	input := []domain.Rejection{
		{Section: "Public Witnessing", Who: "Zed", What: "A"},
		{Section: "Field Ministry", Who: "Zed", What: "B"},
		{Section: "Field Ministry", Who: "Amy", What: "Z"},
		{Section: "Field Ministry", Who: "Amy", What: "A"},
	}

	sorted := canonicalize(input)

	if sorted[0].What != "A" || sorted[1].What != "Z" ||
		sorted[2].Section != "Field Ministry" || sorted[3].Section != "Public Witnessing" {
		t.Fatalf("unexpected canonical order: %#v", sorted)
	}
}

func TestRejectionCache_HasChanges_DifferentSection(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	rejections2 := []domain.Rejection{
		{Section: "Mechanical Parts", Who: "John", What: "Test", When: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different Section should return true")
	}
}

func TestRejectionCache_HasChanges_DifferentWho(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	rejections2 := []domain.Rejection{
		{Section: "Field Ministry", Who: "Jane", What: "Test", When: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different Who should return true")
	}
}

func TestRejectionCache_HasChanges_DifferentWhat(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	rejections2 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Different", When: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if !c.HasChanges(rejections2) {
		t.Error("different What should return true")
	}
}

func TestRejectionCache_HasChanges_SameWhenDifferent(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	rejections2 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "02/03/2026"},
	}

	c.HasChanges(rejections1)

	if c.HasChanges(rejections2) {
		t.Error("When is not compared, so should return false")
	}
}

func TestRejectionCache_HasChanges_EmptyToNonEmpty(t *testing.T) {
	c := New()
	c.HasChanges([]domain.Rejection{})

	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}

	if !c.HasChanges(rejections) {
		t.Error("empty to non-empty should return true")
	}
}

func TestRejectionCache_HasChanges_NonEmptyToEmpty(t *testing.T) {
	c := New()
	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	c.HasChanges(rejections)

	if !c.HasChanges([]domain.Rejection{}) {
		t.Error("non-empty to empty should return true (different count)")
	}
}

func TestRejectionCache_LastCheck(t *testing.T) {
	c := New()

	if !c.LastCheck().IsZero() {
		t.Error("LastCheck should be zero for new cache")
	}

	c.HasChanges([]domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	})

	if c.LastCheck().IsZero() {
		t.Error("LastCheck should be set after HasChanges")
	}
}

func TestRejectionCache_Reset(t *testing.T) {
	c := New()
	rejections := []domain.Rejection{{Section: "Field Ministry", Who: "A", What: "B"}}
	if !c.HasChanges(rejections) {
		t.Fatal("first snapshot should be a change")
	}
	c.Reset()
	if !c.HasChanges(rejections) {
		t.Fatal("reset snapshot should be eligible again")
	}
}

func TestRejectionCache_ConcurrentAccess(t *testing.T) {
	c := New()
	done := make(chan bool)

	go func() {
		for range 100 {
			c.HasChanges([]domain.Rejection{
				{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
			})
		}
		done <- true
	}()

	go func() {
		for range 100 {
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

func TestRejectionCache_MultipleRejectionsOrderIsStable(t *testing.T) {
	c := New()
	rejections1 := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test1", When: "01/03/2026"},
		{Section: "Field Ministry", Who: "Jane", What: "Test2", When: "02/03/2026"},
	}
	rejections2 := []domain.Rejection{
		{Section: "Field Ministry", Who: "Jane", What: "Test2", When: "02/03/2026"},
		{Section: "Field Ministry", Who: "John", What: "Test1", When: "01/03/2026"},
	}

	c.HasChanges(rejections1)

	if c.HasChanges(rejections2) {
		t.Error("different order should not be detected as a change")
	}
}
