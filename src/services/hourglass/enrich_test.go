package hourglass

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserDisplayNameAndLookup(t *testing.T) {
	users := IndexUsersByID([]User{
		{ID: 1, Firstname: "Ada", Lastname: "Lovelace", Descriptor: "Ada Lovelace"},
		{ID: 2, Firstname: "Grace", Lastname: "Hopper"},
		{ID: 3},
	})
	assert.Equal(t, "Ada Lovelace", LookupUserName(users, 1))
	assert.Equal(t, "Grace Hopper", LookupUserName(users, 2))
	assert.Equal(t, "User 3", LookupUserName(users, 3))
	assert.Equal(t, "User 99", LookupUserName(users, 99))
	assert.Equal(t, "", LookupOptionalUserName(users, nil))
	id := 1
	assert.Equal(t, "Ada Lovelace", LookupOptionalUserName(users, &id))
}

func TestEnrichDeclinedNotifications_FiltersAndSubstitute(t *testing.T) {
	users := IndexUsersByID([]User{{ID: 1, Descriptor: "Ada Lovelace"}})
	notifications := []Notification{
		{ID: 1, Type: "pubwit", Date: "2026-01-02", Part: 50, Classroom: 0, Status: "declined", Assignee: 1},
		{ID: 2, Type: "pubwit", Date: "2026-01-02", Part: 50, Classroom: 0, Status: "complete", Assignee: 9},
		{ID: 3, Type: "ava", Date: "2026-01-03", Part: 1, Status: "pending", Assignee: 1},
		{ID: 4, Type: "mm", Date: "2026-01-04", Part: 100, Flag: 10, Status: "declined", Assignee: 1},
	}
	partTitles := map[int]string{100: "Bible Reading"}

	got := EnrichDeclinedNotifications(notifications, users, partTitles)
	require.Len(t, got, 2)

	assert.Equal(t, "pubwit", got[0].Type)
	assert.Equal(t, "Ada Lovelace", got[0].AssigneeName)
	assert.True(t, got[0].SubstituteAssigned)
	assert.Equal(t, "Public Witnessing", got[0].Assignment)

	assert.Equal(t, "mm", got[1].Type)
	assert.Equal(t, "Bible Reading", got[1].Assignment)
	assert.False(t, got[1].SubstituteAssigned)
}

func TestEnrichDeclinedNotifications_NilTitlesUsesFlagName(t *testing.T) {
	got := EnrichDeclinedNotifications([]Notification{{
		ID: 1, Type: "mm", Status: "declined", Flag: 30, Assignee: 5,
	}}, nil, nil)
	require.Len(t, got, 1)
	assert.Equal(t, "Student", got[0].Assignment)
	assert.Equal(t, "User 5", got[0].AssigneeName)
}

func TestHasSubstitute_IgnoresSameAssigneeAndMismatchedSlots(t *testing.T) {
	declined := Notification{ID: 1, Type: "ava", Date: "d", Part: 1, Classroom: 1, Status: "declined", Assignee: 1}
	all := []Notification{
		declined,
		{ID: 2, Type: "ava", Date: "d", Part: 1, Classroom: 1, Status: "pending", Assignee: 1},
		{ID: 3, Type: "ava", Date: "other", Part: 1, Classroom: 1, Status: "complete", Assignee: 2},
		{ID: 4, Type: "ava", Date: "d", Part: 2, Classroom: 1, Status: "complete", Assignee: 2},
	}
	assert.False(t, HasSubstitute(all, declined))
}

func TestEnrichAVAttendantsAndWMAndPrivileges(t *testing.T) {
	id := 1
	users := IndexUsersByID([]User{{ID: 1, Descriptor: "Ada Lovelace"}})

	av := EnrichAVAttendants([]AVAttendant{{
		ID: 9, Type: "mics", Assignee: &id, Date: "2026-01-01", Status: "Aceito", Confirmed: true,
	}, {
		ID: 10, Type: "stage", Date: "2026-01-01", Status: "Esperando",
	}}, users)
	require.Len(t, av, 2)
	assert.Equal(t, "Ada Lovelace", av[0].AssigneeName)
	assert.Equal(t, "Aceito", av[0].Status)
	assert.Equal(t, "mics", av[0].Type)
	assert.Empty(t, av[1].AssigneeName)

	wm := EnrichWMAssignments([]WMAssignment{{
		ID: 1, Role: "speaker", Assignee: &id, Status: "Recusado", Date: "2026-01-04",
	}}, users)
	require.Len(t, wm, 1)
	assert.Equal(t, "Ada Lovelace", wm[0].AssigneeName)
	assert.Equal(t, "Recusado", wm[0].Status)

	priv := EnrichPrivileges([]PrivilegeAssignment{{
		ID: 2, Type: "prayers", Assignee: &id, Status: "NaoEnviado", Date: "2026-01-04",
	}}, users)
	require.Len(t, priv, 1)
	assert.Equal(t, "Ada Lovelace", priv[0].AssigneeName)
	assert.Equal(t, "NaoEnviado", priv[0].Status)
}

func TestEnrichMeetingsWithAssignments(t *testing.T) {
	users := IndexUsersByID([]User{{ID: 1, Descriptor: "Ada Lovelace"}})
	meetings := []Meeting{{
		Date: "2026-01-01", LGroup: 1,
		TGW: []MeetingPart{{ID: 100, Title: "Bible Reading"}},
	}}
	notifications := []Notification{
		{ID: 1, Type: "mm", Date: "2026-01-01", Part: 100, Status: "declined", Assignee: 1},
		{ID: 2, Type: "mm", Date: "2026-01-08", Part: 200, Flag: 20, Status: "pending", Assignee: 1},
		{ID: 3, Type: "ava", Date: "2026-01-01", Part: 1, Status: "declined", Assignee: 1},
	}

	got := EnrichMeetingsWithAssignments(meetings, notifications, users)
	require.Len(t, got, 2)

	assert.Equal(t, "2026-01-01", got[0].Date)
	require.Len(t, got[0].Assignments, 1)
	assert.Equal(t, "Bible Reading", got[0].Assignments[0].Title)
	assert.Equal(t, "declined", got[0].Assignments[0].Status)
	assert.Equal(t, "Ada Lovelace", got[0].Assignments[0].AssigneeName)

	assert.Equal(t, "2026-01-08", got[1].Date)
	require.Len(t, got[1].Assignments, 1)
	assert.Equal(t, "Speaker/Chairman", got[1].Assignments[0].Title)
}

func TestEnrichMeetingsWithAssignments_ScopesRowsToMeetingParts(t *testing.T) {
	users := IndexUsersByID([]User{
		{ID: 1, Descriptor: "Ada Lovelace"},
		{ID: 2, Descriptor: "Grace Hopper"},
	})
	// Two language groups on the same date with disjoint part IDs.
	meetings := []Meeting{
		{
			Date: "2026-01-01", LGroup: 10,
			TGW: []MeetingPart{{ID: 100, Title: "Bible Reading A"}},
			FM:  []MeetingPart{{ID: 101, Title: "Initial Call A"}},
		},
		{
			Date: "2026-01-01", LGroup: 20,
			TGW: []MeetingPart{{ID: 200, Title: "Bible Reading B"}},
			LAC: []MeetingPart{{ID: 201, Title: "Needs B"}},
		},
	}
	notifications := []Notification{
		{ID: 1, Type: "mm", Date: "2026-01-01", Part: 100, Status: "declined", Assignee: 1},
		{ID: 2, Type: "mm", Date: "2026-01-01", Part: 101, Status: "pending", Assignee: 1},
		{ID: 3, Type: "mm", Date: "2026-01-01", Part: 200, Status: "complete", Assignee: 2},
		{ID: 4, Type: "mm", Date: "2026-01-01", Part: 201, Status: "declined", Assignee: 2},
		// Part absent from both meetings stays as a leftover notification-only row.
		{ID: 5, Type: "mm", Date: "2026-01-01", Part: 999, Flag: 20, Status: "pending", Assignee: 1},
	}

	got := EnrichMeetingsWithAssignments(meetings, notifications, users)
	require.Len(t, got, 3)

	assert.Equal(t, 10, got[0].LGroup)
	require.Len(t, got[0].Assignments, 2)
	assert.Equal(t, 100, got[0].Assignments[0].PartID)
	assert.Equal(t, 101, got[0].Assignments[1].PartID)
	assert.Equal(t, "Bible Reading A", got[0].Assignments[0].Title)

	assert.Equal(t, 20, got[1].LGroup)
	require.Len(t, got[1].Assignments, 2)
	assert.Equal(t, 200, got[1].Assignments[0].PartID)
	assert.Equal(t, 201, got[1].Assignments[1].PartID)
	assert.Equal(t, "Needs B", got[1].Assignments[1].Title)

	// Leftover part 999 must not be duplicated onto either meeting.
	assert.Equal(t, "2026-01-01", got[2].Date)
	assert.Zero(t, got[2].LGroup)
	require.Len(t, got[2].Assignments, 1)
	assert.Equal(t, 999, got[2].Assignments[0].PartID)
	assert.Equal(t, "Speaker/Chairman", got[2].Assignments[0].Title)
}

func TestPartTitlesFromMeetings(t *testing.T) {
	titles := PartTitlesFromMeetings([]Meeting{{
		TGW: []MeetingPart{{ID: 1, Title: "A"}},
		FM:  []MeetingPart{{ID: 2, Title: "B"}},
		LAC: []MeetingPart{{ID: 3, Title: "C"}},
	}})
	assert.Equal(t, "A", titles[1])
	assert.Equal(t, "B", titles[2])
	assert.Equal(t, "C", titles[3])
}
