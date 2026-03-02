package api

import "time"

// User represents a user in the Hourglass system.
type User struct {
	ID         int       `json:"id"`
	Firstname  string    `json:"firstname"`
	Lastname   string    `json:"lastname"`
	Descriptor string    `json:"descriptor"` // Display name
	Appt       string    `json:"appt"`       // Appointment (e.g., "Elder", "MS")
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// UsersResponse represents the response from the users endpoint.
type UsersResponse struct {
	Users []User `json:"users"`
}

// AVAttendant represents a mechanical assignment (Áudio/Vídeo & Indicadores).
type AVAttendant struct {
	ID        int       `json:"id"`
	Type      string    `json:"type"`     // e.g., "video", "console", "mics", "attendant", "security_attendant", "stage"
	Assignee  *int      `json:"assignee"` // Nullable - user ID or null if vacant
	Slot      int       `json:"slot"`
	Date      string    `json:"date"` // Format: "2026-03-01"
	Extra     bool      `json:"extra"`
	Confirmed bool      `json:"confirmed"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Meeting represents a midweek meeting schedule.
type Meeting struct {
	Date   string        `json:"date"`
	LGroup int           `json:"lgroup"`
	TGW    []MeetingPart `json:"tgw"` // Treasures from God's Word
	FM     []MeetingPart `json:"fm"`  // Field Ministry
	LAC    []MeetingPart `json:"lac"` // Living as Christians
}

// MeetingPart represents a single part within a meeting section.
type MeetingPart struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Info  string `json:"info"`
	Time  string `json:"time"`
	Type  string `json:"type"`
}

// MeetingSection represents participants for a meeting part.
type MeetingSection struct {
	ID           int           `json:"id"`
	Participants []Participant `json:"participants"`
}

// Participant represents a person assigned to a meeting section.
type Participant struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Type      string `json:"type"` // e.g., "tgw", "fm", "lac"
	Slot      int    `json:"slot"`
	Optional  bool   `json:"optional"`
	Confirmed bool   `json:"confirmed"`
}

// AssignmentStatus represents the status of an assignment.
type AssignmentStatus string

const (
	StatusAceito     AssignmentStatus = "Aceito"
	StatusEsperando  AssignmentStatus = "Esperando"
	StatusAusencia   AssignmentStatus = "Ausencia"
	StatusNaoEnviado AssignmentStatus = "NaoEnviado"
	StatusRecusado   AssignmentStatus = "Recusado"
)

// Notification represents a notification/assignment status from Hourglass.
type Notification struct {
	ID             int    `json:"id"`
	CongregationID int    `json:"congregationId"`
	Date           string `json:"date"`
	Classroom      int    `json:"classroom"`
	Part           int    `json:"part"`
	Type           string `json:"type"` // e.g., "pubwit", "avattendant"
	Flag           int    `json:"flag"`
	Status         string `json:"status"` // "pending", "complete", "declined"
	Assignee       int    `json:"assignee"`
}

// NotificationStatus represents possible notification statuses.
type NotificationStatus string

const (
	StatusPending  NotificationStatus = "pending"
	StatusComplete NotificationStatus = "complete"
	StatusDeclined NotificationStatus = "declined"
)
