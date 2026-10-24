package hourglass

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCleanupPlan_RotatesDeterministically(t *testing.T) {
	ada, bob, cy := 1, 2, 3
	month := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	plan, err := BuildCleanupPlan(month, []AVAttendant{
		{ID: 1, Date: "2026-09-01", Type: "attendant", Assignee: &ada},
		{ID: 2, Date: "2026-10-01", Type: "attendant", Assignee: &ada},
		{ID: 3, Date: "2026-10-08", Type: "attendant", Assignee: &bob},
	}, []AVAttendant{
		{ID: 11, Date: "2026-11-01", Type: "attendant", Slot: 1, UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 12, Date: "2026-11-08", Type: "attendant", Slot: 1, UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}, []User{{ID: ada, Descriptor: "Ada"}, {ID: bob, Descriptor: "Bob"}, {ID: cy, Descriptor: "Cy"}}, CleanupRules{
		EligibleUserIDs:               []int{ada, bob, cy},
		Types:                         []string{"attendant"},
		MaxAssignmentsPerUserPerMonth: 1,
		DisallowSameDate:              true,
	}, 3, 0)
	require.NoError(t, err)
	require.Len(t, plan.Mutations, 2)
	assert.Equal(t, cy, plan.Mutations[0].AssigneeID)
	assert.Equal(t, bob, plan.Mutations[1].AssigneeID)
	assert.True(t, plan.DryRun)
	assert.NotEmpty(t, plan.Confirm)
}

func TestBuildCleanupPlan_SkipsUnsafeRowsAndNoCandidate(t *testing.T) {
	one := 1
	month := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	plan, err := BuildCleanupPlan(month, nil, []AVAttendant{
		{ID: 1, Date: "2026-11-01", Type: "security_attendant", UpdatedAt: time.Now()},
		{ID: 2, Date: "2026-11-08", Type: "attendant", Published: true, UpdatedAt: time.Now()},
		{ID: 3, Date: "2026-11-15", Type: "attendant", Extra: true, UpdatedAt: time.Now()},
		{ID: 4, Date: "2026-11-22", Type: "attendant", UpdatedAt: time.Now()},
		{ID: 5, Date: "2026-11-29", Type: "attendant", Assignee: &one, UpdatedAt: time.Now()},
	}, []User{{ID: one, Descriptor: "One"}}, CleanupRules{
		EligibleUserIDs:               []int{one},
		Types:                         []string{"attendant"},
		MaxAssignmentsPerUserPerMonth: 1,
	}, 3, 0)
	require.NoError(t, err)
	assert.Empty(t, plan.Mutations)
	assert.Len(t, plan.Skipped, 5)
	assert.Equal(t, "type_not_allowed", plan.Skipped[0].Reason)
	assert.Equal(t, "published", plan.Skipped[1].Reason)
	assert.Equal(t, "extra", plan.Skipped[2].Reason)
	assert.Equal(t, "no_eligible_candidate", plan.Skipped[3].Reason)
	assert.Equal(t, "already_assigned", plan.Skipped[4].Reason)
}

func TestBuildCleanupPlan_RejectsInvalidRules(t *testing.T) {
	_, err := BuildCleanupPlan(time.Now(), nil, nil, nil, CleanupRules{}, 3, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "eligible_user_ids")
}

func TestCleanupPlanConfirmToken_ChangesWhenInputChanges(t *testing.T) {
	base := CleanupPlan{Month: "2026-11", LGroup: 0, RulesDigest: "rules", Mutations: []CleanupMutation{{SlotID: 1, AssigneeID: 2, UpdatedAt: "2026-10-01T00:00:00Z"}}}
	a := CleanupPlanConfirmToken(base)
	b := CleanupPlanConfirmToken(base)
	assert.Equal(t, a, b)
	base.Mutations[0].AssigneeID = 3
	assert.NotEqual(t, a, CleanupPlanConfirmToken(base))
}
