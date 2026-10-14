package hourglass

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignSwapConfirmToken_DeterministicAndStale(t *testing.T) {
	a := AssignSwapConfirmToken(9, 1, 2, "2026-01-01T00:00:00Z")
	b := AssignSwapConfirmToken(9, 1, 2, "2026-01-01T00:00:00Z")
	assert.Equal(t, a, b)
	assert.Len(t, a, 32)

	staleVersion := AssignSwapConfirmToken(9, 1, 2, "2026-01-02T00:00:00Z")
	assert.NotEqual(t, a, staleVersion)

	staleTarget := AssignSwapConfirmToken(9, 1, 3, "2026-01-01T00:00:00Z")
	assert.NotEqual(t, a, staleTarget)
}

func TestAssignSwapVersion(t *testing.T) {
	ts := time.Date(2026, 3, 1, 12, 0, 0, 123, time.UTC)
	row := AVAttendant{Date: "2026-03-01", UpdatedAt: ts}
	assert.Equal(t, ts.Format(time.RFC3339Nano), AssignSwapVersion(row))

	empty := AVAttendant{Date: "2026-03-02"}
	assert.Equal(t, "2026-03-02", AssignSwapVersion(empty))
}

func TestBuildAssignSwapPreview(t *testing.T) {
	cur := 10
	row := AVAttendant{
		ID: 5, Type: "mics", Slot: 1, Date: "2026-03-01",
		Assignee: &cur, UpdatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	}
	users := map[int]User{
		10: {ID: 10, Descriptor: "Ada"},
		20: {ID: 20, Descriptor: "Bob"},
	}
	preview := BuildAssignSwapPreview(row, users[20], 0, users)
	assert.True(t, preview.DryRun)
	assert.Equal(t, 5, preview.SlotID)
	assert.Equal(t, "Ada", preview.Before.AssigneeName)
	assert.Equal(t, "Bob", preview.After.AssigneeName)
	require.NotNil(t, preview.After.AssigneeID)
	assert.Equal(t, 20, *preview.After.AssigneeID)
	assert.Equal(t, AssignSwapConfirmToken(5, 10, 20, AssignSwapVersion(row)), preview.Confirm)
}

func TestResolveUserRef(t *testing.T) {
	users := []User{
		{ID: 1, Firstname: "Ada", Lastname: "Lovelace", Descriptor: "Ada Lovelace"},
		{ID: 2, Firstname: "Bob", Lastname: "Builder", Descriptor: "Bob"},
		{ID: 3, Firstname: "Bob", Lastname: "Other", Descriptor: "Bobby"},
	}

	u, err := ResolveUserRef("1", users)
	require.NoError(t, err)
	assert.Equal(t, 1, u.ID)

	u, err = ResolveUserRef("Bob", users)
	require.NoError(t, err)
	assert.Equal(t, 2, u.ID)

	_, err = ResolveUserRef("999", users)
	assert.ErrorContains(t, err, "unknown user id")

	_, err = ResolveUserRef("nobody", users)
	assert.ErrorContains(t, err, "unknown user")

	_, err = ResolveUserRef("", users)
	assert.ErrorContains(t, err, "required")

	// Ambiguous full name match
	users = append(users, User{ID: 4, Firstname: "Ada", Lastname: "Lovelace", Descriptor: "Other Ada"})
	_, err = ResolveUserRef("Ada Lovelace", users)
	assert.ErrorContains(t, err, "ambiguous")
}

func TestDefaultAssignSwapRange(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	from, until := DefaultAssignSwapRange(now)
	assert.Equal(t, "2026-05-16", from)
	assert.Equal(t, "2027-06-15", until)
}

func TestFindAVAttendantByID_AndSaveAssignee(t *testing.T) {
	assignee := 10
	newAssignee := 20
	updatedAt := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	var putCount int
	var putBody []byte
	var putPath string
	var putQuery string
	var seenMethods []string

	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethods = append(seenMethods, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			assert.Equal(t, "3", r.URL.Query().Get("lgroup"))
			_ = json.NewEncoder(w).Encode([]AVAttendant{{
				ID: 42, Type: "mics", Slot: 0, Date: "2026-03-01",
				Assignee: &assignee, UpdatedAt: updatedAt,
			}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(UsersResponse{Users: []User{
				{ID: 10, Descriptor: "Ada"},
				{ID: 20, Descriptor: "Bob"},
			}})
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/scheduling/av_attendant/42"):
			putCount++
			putPath = r.URL.Path
			putQuery = r.URL.RawQuery
			body, _ := io.ReadAll(r.Body)
			putBody = body
			var payload map[string]any
			require.NoError(t, json.Unmarshal(body, &payload))
			_, hasPublished := payload["published"]
			_, hasNotify := payload["notify"]
			assert.False(t, hasPublished)
			assert.False(t, hasNotify)
			assert.Equal(t, float64(20), payload["assignee"])
			_ = json.NewEncoder(w).Encode(AVAttendant{
				ID: 42, Type: "mics", Slot: 0, Date: "2026-03-01",
				Assignee: &newAssignee, UpdatedAt: updatedAt.Add(time.Minute),
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))

	client := NewClient()
	client.SetBaseURL(server.URL + "/api/v0.2")

	row, err := client.FindAVAttendantByID(42, "2026-03-01", "2026-03-07", 3)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, 42, row.ID)

	_, err = client.FindAVAttendantByID(99, "2026-03-01", "2026-03-07", 3)
	assert.ErrorContains(t, err, "unknown slot")

	next := *row
	next.Assignee = &newAssignee
	got, err := client.SaveAVAttendantAssignee(next, 3)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, putCount)
	assert.Contains(t, putPath, "/scheduling/av_attendant/42")
	assert.Equal(t, "lgroup=3", putQuery)
	assert.Contains(t, string(putBody), `"assignee":20`)
	assert.NotContains(t, string(putBody), "published")
	assert.NotContains(t, string(putBody), "notify")

	for _, m := range seenMethods {
		assert.NotContains(t, strings.ToLower(m), "publish")
		assert.NotContains(t, strings.ToLower(m), "notify")
		assert.NotContains(t, strings.ToLower(m), "send")
	}

	_, err = client.SaveAVAttendantAssignee(AVAttendant{}, 0)
	assert.ErrorContains(t, err, "assignment id is required")
}

func TestSaveAVAttendantAssignee_ErrorStatus(t *testing.T) {
	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	client := NewClient()
	client.SetBaseURL(server.URL + "/api/v0.2")
	id := 1
	_, err := client.SaveAVAttendantAssignee(AVAttendant{ID: 7, Assignee: &id, Date: "2026-01-01"}, 0)
	assert.ErrorContains(t, err, "unexpected status code: 400")
}

func TestSaveAVAttendantAssignee_DecodeError(t *testing.T) {
	server := newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	client := NewClient()
	client.SetBaseURL(server.URL + "/api/v0.2")
	id := 1
	_, err := client.SaveAVAttendantAssignee(AVAttendant{ID: 7, Assignee: &id, Date: "2026-01-01", Type: "mics"}, 0)
	assert.ErrorContains(t, err, "failed to decode response")
}

func TestAssigneeIDOrZero(t *testing.T) {
	assert.Equal(t, 0, assigneeIDOrZero(nil))
	n := 5
	assert.Equal(t, 5, assigneeIDOrZero(&n))
}
