package hourglass

import "fmt"

// IndexUsersByID builds a lookup map from user ID to User.
func IndexUsersByID(users []User) map[int]User {
	index := make(map[int]User, len(users))
	for _, user := range users {
		index[user.ID] = user
	}
	return index
}

// UserDisplayName returns the preferred display name for a user.
func UserDisplayName(user User) string {
	if user.Descriptor != "" {
		return user.Descriptor
	}
	name := fmt.Sprintf("%s %s", user.Firstname, user.Lastname)
	if name == " " {
		return fmt.Sprintf("User %d", user.ID)
	}
	return name
}

// LookupUserName resolves a user ID through the index.
func LookupUserName(index map[int]User, userID int) string {
	if user, ok := index[userID]; ok {
		return UserDisplayName(user)
	}
	return fmt.Sprintf("User %d", userID)
}

// LookupOptionalUserName resolves a nullable user ID.
func LookupOptionalUserName(index map[int]User, userID *int) string {
	if userID == nil {
		return ""
	}
	return LookupUserName(index, *userID)
}

// PartTitlesFromMeetings maps midweek part IDs to titles.
func PartTitlesFromMeetings(meetings []Meeting) map[int]string {
	titles := make(map[int]string)
	for _, meeting := range meetings {
		for _, part := range meeting.TGW {
			titles[part.ID] = part.Title
		}
		for _, part := range meeting.FM {
			titles[part.ID] = part.Title
		}
		for _, part := range meeting.LAC {
			titles[part.ID] = part.Title
		}
	}
	return titles
}

func assignmentTitle(notif Notification, partTitles map[int]string) string {
	if title := partTitles[notif.Part]; title != "" {
		return title
	}
	if notif.Type == "mm" {
		return getMidweekFlagName(notif.Flag)
	}
	return getFriendlyTypeName(notif.Type)
}

// HasSubstitute reports whether another pending/complete notification covers the
// same slot after a decline (a substitute assignment).
func HasSubstitute(all []Notification, declined Notification) bool {
	for _, candidate := range all {
		if candidate.ID == declined.ID {
			continue
		}
		if candidate.Date != declined.Date ||
			candidate.Part != declined.Part ||
			candidate.Type != declined.Type ||
			candidate.Classroom != declined.Classroom {
			continue
		}
		if candidate.Assignee == declined.Assignee {
			continue
		}
		switch NotificationStatus(candidate.Status) {
		case StatusPending, StatusComplete:
			return true
		}
	}
	return false
}

// EnrichDeclinedNotifications keeps declined rows and adds name/assignment/substitute fields.
func EnrichDeclinedNotifications(
	notifications []Notification,
	users map[int]User,
	partTitles map[int]string,
) []DeclinedAssignment {
	if partTitles == nil {
		partTitles = map[int]string{}
	}
	out := make([]DeclinedAssignment, 0, len(notifications))
	for _, notif := range notifications {
		if NotificationStatus(notif.Status) != StatusDeclined {
			continue
		}
		out = append(out, DeclinedAssignment{
			ID:                 notif.ID,
			Type:               notif.Type,
			Date:               notif.Date,
			Part:               notif.Part,
			Classroom:          notif.Classroom,
			Assignment:         assignmentTitle(notif, partTitles),
			AssigneeID:         notif.Assignee,
			AssigneeName:       LookupUserName(users, notif.Assignee),
			Status:             notif.Status,
			SubstituteAssigned: HasSubstitute(notifications, notif),
		})
	}
	return out
}

// EnrichAVAttendants resolves assignee names while preserving AV status/type fields.
func EnrichAVAttendants(attendants []AVAttendant, users map[int]User) []AVAttendantView {
	out := make([]AVAttendantView, 0, len(attendants))
	for _, row := range attendants {
		out = append(out, AVAttendantView{
			ID:           row.ID,
			Type:         row.Type,
			AssigneeID:   row.Assignee,
			AssigneeName: LookupOptionalUserName(users, row.Assignee),
			Slot:         row.Slot,
			Date:         row.Date,
			Extra:        row.Extra,
			Status:       row.Status,
			Confirmed:    row.Confirmed,
			Published:    row.Published,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		})
	}
	return out
}

// EnrichMeetingsWithAssignments attaches per-assignment status rows from mm notifications.
func EnrichMeetingsWithAssignments(
	meetings []Meeting,
	notifications []Notification,
	users map[int]User,
) []MeetingView {
	titles := PartTitlesFromMeetings(meetings)
	byDate := make(map[string][]MMAssignmentView)
	for _, notif := range notifications {
		if notif.Type != "" && notif.Type != "mm" {
			// Keep non-mm rows out of the midweek view when a mixed feed is passed.
			continue
		}
		view := MMAssignmentView{
			Date:         notif.Date,
			PartID:       notif.Part,
			Title:        assignmentTitle(notif, titles),
			Flag:         notif.Flag,
			AssigneeID:   notif.Assignee,
			AssigneeName: LookupUserName(users, notif.Assignee),
			Status:       notif.Status,
			Classroom:    notif.Classroom,
		}
		byDate[notif.Date] = append(byDate[notif.Date], view)
	}

	out := make([]MeetingView, 0, len(meetings))
	seen := make(map[string]bool, len(meetings))
	for _, meeting := range meetings {
		seen[meeting.Date] = true
		out = append(out, MeetingView{
			Date:        meeting.Date,
			LGroup:      meeting.LGroup,
			TGW:         meeting.TGW,
			FM:          meeting.FM,
			LAC:         meeting.LAC,
			Assignments: byDate[meeting.Date],
		})
	}
	// Include notification-only dates so status rows are not dropped.
	for date, assignments := range byDate {
		if seen[date] {
			continue
		}
		out = append(out, MeetingView{
			Date:        date,
			Assignments: assignments,
		})
	}
	return out
}

// EnrichWMAssignments resolves assignee names for WM rows.
func EnrichWMAssignments(rows []WMAssignment, users map[int]User) []WMAssignmentView {
	out := make([]WMAssignmentView, 0, len(rows))
	for _, row := range rows {
		out = append(out, WMAssignmentView{
			ID:           row.ID,
			Role:         row.Role,
			AssigneeID:   row.Assignee,
			AssigneeName: LookupOptionalUserName(users, row.Assignee),
			Status:       row.Status,
			Date:         row.Date,
			Extra:        row.Extra,
			Confirmed:    row.Confirmed,
			Published:    row.Published,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		})
	}
	return out
}

// EnrichPrivileges resolves assignee names for privilege rows.
func EnrichPrivileges(rows []PrivilegeAssignment, users map[int]User) []PrivilegeAssignmentView {
	out := make([]PrivilegeAssignmentView, 0, len(rows))
	for _, row := range rows {
		out = append(out, PrivilegeAssignmentView{
			ID:           row.ID,
			Type:         row.Type,
			AssigneeID:   row.Assignee,
			AssigneeName: LookupOptionalUserName(users, row.Assignee),
			Status:       row.Status,
			Date:         row.Date,
			Confirmed:    row.Confirmed,
			Published:    row.Published,
		})
	}
	return out
}
