package hourglass

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AssignSwapParty is one side of an assignee swap preview.
type AssignSwapParty struct {
	AssigneeID   *int   `json:"assignee_id"`
	AssigneeName string `json:"assignee_name,omitempty"`
	Date         string `json:"date"`
	Type         string `json:"type"`
	Slot         int    `json:"slot"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// AssignSwapResult is the dry-run or applied JSON for assign swap.
type AssignSwapResult struct {
	DryRun  bool            `json:"dry_run"`
	Applied bool            `json:"applied,omitempty"`
	SlotID  int             `json:"slot_id"`
	LGroup  int             `json:"lgroup"`
	Before  AssignSwapParty `json:"before"`
	After   AssignSwapParty `json:"after"`
	Confirm string          `json:"confirm,omitempty"`
}

// AssignSwapVersion returns the version string used in confirm tokens.
// Prefers updated_at so a concurrent edit invalidates a stale preview.
func AssignSwapVersion(row AVAttendant) string {
	if !row.UpdatedAt.IsZero() {
		return row.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return row.Date
}

// AssignSwapConfirmToken derives the dry-run confirm token for a swap.
// Token is deterministic from slot, current assignee, target assignee, and version.
func AssignSwapConfirmToken(slotID int, currentAssignee, targetAssignee int, version string) string {
	payload := fmt.Sprintf("%d|%d|%d|%s", slotID, currentAssignee, targetAssignee, version)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])[:32]
}

func assigneeIDOrZero(id *int) int {
	if id == nil {
		return 0
	}
	return *id
}

// BuildAssignSwapPreview builds a before/after preview with confirm token.
func BuildAssignSwapPreview(row AVAttendant, targetUser User, lgroup int, users map[int]User) AssignSwapResult {
	targetID := targetUser.ID
	version := AssignSwapVersion(row)
	currentID := assigneeIDOrZero(row.Assignee)
	return AssignSwapResult{
		DryRun: true,
		SlotID: row.ID,
		LGroup: lgroup,
		Before: AssignSwapParty{
			AssigneeID:   row.Assignee,
			AssigneeName: LookupOptionalUserName(users, row.Assignee),
			Date:         row.Date,
			Type:         row.Type,
			Slot:         row.Slot,
			UpdatedAt:    version,
		},
		After: AssignSwapParty{
			AssigneeID:   &targetID,
			AssigneeName: LookupUserName(users, targetID),
			Date:         row.Date,
			Type:         row.Type,
			Slot:         row.Slot,
			UpdatedAt:    version,
		},
		Confirm: AssignSwapConfirmToken(row.ID, currentID, targetID, version),
	}
}

// ResolveUserRef resolves a user id or case-insensitive descriptor/name.
func ResolveUserRef(ref string, users []User) (User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return User{}, fmt.Errorf("target user is required")
	}
	if id, err := strconv.Atoi(ref); err == nil {
		for _, u := range users {
			if u.ID == id {
				return u, nil
			}
		}
		return User{}, fmt.Errorf("unknown user id %d", id)
	}
	lower := strings.ToLower(ref)
	var matches []User
	for _, u := range users {
		if strings.ToLower(u.Descriptor) == lower ||
			strings.ToLower(strings.TrimSpace(u.Firstname+" "+u.Lastname)) == lower {
			matches = append(matches, u)
		}
	}
	switch len(matches) {
	case 0:
		return User{}, fmt.Errorf("unknown user %q", ref)
	case 1:
		return matches[0], nil
	default:
		return User{}, fmt.Errorf("ambiguous user %q", ref)
	}
}

// DefaultAssignSwapRange returns a search window when --from/--until are omitted.
func DefaultAssignSwapRange(now time.Time) (from, until string) {
	start := now.UTC().AddDate(0, 0, -30)
	end := now.UTC().AddDate(1, 0, 0)
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}
