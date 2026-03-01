package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, defaultBaseURL, client.baseURL)
}

func TestClient_SetXSRFToken(t *testing.T) {
	client := NewClient()
	client.SetXSRFToken("test-token-123")
	assert.Equal(t, "test-token-123", client.xsrfToken)
}

func TestClient_GetUsers(t *testing.T) {
	expectedUsers := []User{
		{
			ID:         31944,
			Firstname:  "João",
			Lastname:   "Silva",
			Descriptor: "João Silva",
			Appt:       "Elder",
		},
		{
			ID:         31945,
			Firstname:  "Maria",
			Lastname:   "Santos",
			Descriptor: "Maria Santos",
			Appt:       "MS",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/fsreport/users", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UsersResponse{Users: expectedUsers})
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	users, err := client.GetUsers()
	require.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "João", users[0].Firstname)
	assert.Equal(t, "Maria", users[1].Firstname)
}

func TestClient_GetUsers_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	users, err := client.GetUsers()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "unexpected status code: 401")
}

func TestClient_GetAVAttendants(t *testing.T) {
	expected := []AVAttendant{
		{
			ID:        57427,
			Type:      "attendant",
			Assignee:  nil,
			Slot:      1,
			Date:      "2026-03-01",
			Extra:     false,
			Confirmed: false,
			Published: true,
		},
		{
			ID:        57428,
			Type:      "video",
			Assignee:  intPtr(31944),
			Slot:      1,
			Date:      "2026-03-01",
			Extra:     false,
			Confirmed: true,
			Published: true,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/scheduling/av_attendant/2026-03-01_2026-03-07", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	attendants, err := client.GetAVAttendants("2026-03-01", "2026-03-07")
	require.NoError(t, err)
	assert.Len(t, attendants, 2)
	assert.Equal(t, "attendant", attendants[0].Type)
	assert.Nil(t, attendants[0].Assignee)
	assert.Equal(t, 31944, *attendants[1].Assignee)
}

func TestClient_GetMeetings(t *testing.T) {
	expected := []Meeting{
		{
			Date:   "2026-03-02",
			LGroup: 48092,
			TGW: MeetingSection{
				ID: 123,
				Participants: []Participant{
					{
						ID:        1,
						UserID:    31944,
						Type:      "tgw",
						Slot:      1,
						Optional:  false,
						Confirmed: true,
					},
				},
			},
			FM: MeetingSection{
				ID: 124,
				Participants: []Participant{
					{
						ID:        2,
						UserID:    31945,
						Type:      "fm",
						Slot:      1,
						Optional:  false,
						Confirmed: false,
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/scheduling/mm/meeting/2026-03-01_2026-03-07", r.URL.Path)
		assert.Equal(t, "lgroup=48092&no_subs=true", r.URL.RawQuery)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	meetings, err := client.GetMeetings("2026-03-01", "2026-03-07", 48092)
	require.NoError(t, err)
	assert.Len(t, meetings, 1)
	assert.Equal(t, "2026-03-02", meetings[0].Date)
	assert.Equal(t, 1, len(meetings[0].TGW.Participants))
}

func TestClient_GetMeetings_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	meetings, err := client.GetMeetings("2026-03-01", "2026-03-07", 48092)
	assert.Error(t, err)
	assert.Nil(t, meetings)
}

func TestClient_WithXSRFToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Hourglass-XSRF-Token")
		assert.Equal(t, "KizSvAQQ4B6lCkHNGagusFtX5nRjlbVZ", token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(UsersResponse{Users: []User{}})
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	client.SetXSRFToken("KizSvAQQ4B6lCkHNGagusFtX5nRjlbVZ")

	_, err := client.GetUsers()
	require.NoError(t, err)
}

func TestClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	client.httpClient.Timeout = 50 * time.Millisecond

	_, err := client.GetUsers()
	assert.Error(t, err)
}

// Helper function
func intPtr(i int) *int {
	return &i
}

func TestClient_GetUsers_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	users, err := client.GetUsers()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestClient_GetAVAttendants_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	attendants, err := client.GetAVAttendants("2026-03-01", "2026-03-07")
	assert.Error(t, err)
	assert.Nil(t, attendants)
}

func TestClient_GetAVAttendants_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "forbidden"}`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	attendants, err := client.GetAVAttendants("2026-03-01", "2026-03-07")
	assert.Error(t, err)
	assert.Nil(t, attendants)
	assert.Contains(t, err.Error(), "unexpected status code: 403")
}

func TestClient_GetMeetings_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	meetings, err := client.GetMeetings("2026-03-01", "2026-03-07", 48092)
	assert.Error(t, err)
	assert.Nil(t, meetings)
}

func TestModels(t *testing.T) {
	// Test User model
	user := User{
		ID:         1,
		Firstname:  "Test",
		Lastname:   "User",
		Descriptor: "Test User",
		Appt:       "Elder",
	}
	assert.Equal(t, 1, user.ID)
	assert.Equal(t, "Test", user.Firstname)

	// Test AVAttendant with nil Assignee
	attendant := AVAttendant{
		ID:        1,
		Type:      "video",
		Assignee:  nil,
		Slot:      1,
		Date:      "2026-03-01",
		Extra:     false,
		Confirmed: false,
		Published: true,
	}
	assert.Nil(t, attendant.Assignee)

	// Test AssignmentStatus constants
	assert.Equal(t, AssignmentStatus("Aceito"), StatusAceito)
	assert.Equal(t, AssignmentStatus("Recusado"), StatusRecusado)
}

func TestClient_InvalidURL(t *testing.T) {
	client := NewClient()
	client.baseURL = "://invalid-url"

	_, err := client.GetUsers()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestClient_GetAVAttendants_InvalidURL(t *testing.T) {
	client := NewClient()
	client.baseURL = "://invalid"

	_, err := client.GetAVAttendants("2026-03-01", "2026-03-07")
	assert.Error(t, err)
}

func TestClient_GetMeetings_InvalidURL(t *testing.T) {
	client := NewClient()
	client.baseURL = "://invalid"

	_, err := client.GetMeetings("2026-03-01", "2026-03-07", 48092)
	assert.Error(t, err)
}

func TestClient_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"users": []}`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	users, err := client.GetUsers()
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestClient_GetUsers_NetworkError(t *testing.T) {
	client := NewClient()
	client.baseURL = "http://localhost:1"

	_, err := client.GetUsers()
	assert.Error(t, err)
}

func TestClient_GetAVAttendants_NetworkError(t *testing.T) {
	client := NewClient()
	client.baseURL = "http://localhost:1"

	_, err := client.GetAVAttendants("2026-03-01", "2026-03-07")
	assert.Error(t, err)
}

func TestClient_GetMeetings_NetworkError(t *testing.T) {
	client := NewClient()
	client.baseURL = "http://localhost:1"

	_, err := client.GetMeetings("2026-03-01", "2026-03-07", 48092)
	assert.Error(t, err)
}

func TestClient_GetNotifications(t *testing.T) {
	expected := []Notification{
		{
			ID:             1,
			CongregationID: 48092,
			Date:           "2026-03-01",
			Type:           "pubwit",
			Status:         "declined",
			Assignee:       123,
			Part:           100,
		},
		{
			ID:             2,
			CongregationID: 48092,
			Date:           "2026-03-02",
			Type:           "pubwit",
			Status:         "pending",
			Assignee:       456,
			Part:           101,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/scheduling/notifications/2026-03-01_2026-03-31/pubwit", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)


		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	notifications, err := client.GetNotifications("2026-03-01", "2026-03-31", "pubwit")
	require.NoError(t, err)
	assert.Len(t, notifications, 2)
	assert.Equal(t, "declined", notifications[0].Status)
	assert.Equal(t, "pending", notifications[1].Status)
}

func TestClient_GetNotifications_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	notifications, err := client.GetNotifications("2026-03-01", "2026-03-31", "pubwit")
	assert.Error(t, err)
	assert.Nil(t, notifications)
	assert.Contains(t, err.Error(), "unexpected status code: 500")
}

func TestClient_GetNotifications_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	notifications, err := client.GetNotifications("2026-03-01", "2026-03-31", "pubwit")
	assert.Error(t, err)
	assert.Nil(t, notifications)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestClient_GetNotifications_InvalidURL(t *testing.T) {
	client := NewClient()
	client.baseURL = "://invalid"

	_, err := client.GetNotifications("2026-03-01", "2026-03-31", "pubwit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestClient_GetNotifications_NetworkError(t *testing.T) {
	client := NewClient()
	client.baseURL = "http://localhost:1"

	_, err := client.GetNotifications("2026-03-01", "2026-03-31", "pubwit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}
