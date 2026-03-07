package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIAnalyzer(t *testing.T) {
	client := NewClient()
	analyzer := NewAPIAnalyzer(client)

	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.client)
	assert.NotNil(t, analyzer.userCache)
	assert.Equal(t, 48092, analyzer.congregationID)
}

func TestAPIAnalyzer_SetCongregationID(t *testing.T) {
	client := NewClient()
	analyzer := NewAPIAnalyzer(client)

	analyzer.SetCongregationID(12345)
	assert.Equal(t, 12345, analyzer.congregationID)
}

func TestAPIAnalyzer_AnalyzeSection_UnknownSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(UsersResponse{Users: []User{}})
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("UnknownSection")
	require.NoError(t, err)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "unknown section")
}

func TestAPIAnalyzer_AnalyzeSection_PartesMecanicas(t *testing.T) {
	users := []User{
		{ID: 1, Firstname: "João", Lastname: "Silva", Descriptor: "João Silva"},
	}

	notifications := []Notification{
		// Declined - should be detected as rejection
		{
			ID:             1,
			CongregationID: 48092,
			Date:           "2026-03-01",
			Type:           "avattendant",
			Status:         "declined",
			Assignee:       1,
			Part:           123,
		},
		// Pending - should NOT be detected
		{
			ID:             2,
			CongregationID: 48092,
			Date:           "2026-03-02",
			Type:           "avattendant",
			Status:         "pending",
			Assignee:       1,
			Part:           124,
		},
		// Complete - should NOT be detected
		{
			ID:             3,
			CongregationID: 48092,
			Date:           "2026-03-03",
			Type:           "avattendant",
			Status:         "complete",
			Assignee:       1,
			Part:           125,
		},
	}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/ava", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Partes Mecânicas")
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, "Partes Mecânicas", result.Secao)
	assert.Equal(t, 1, result.Total) // Only 1 declined
	assert.Len(t, result.Rejeicoes, 1)
	assert.Equal(t, "João Silva", result.Rejeicoes[0].Quem)
	assert.Contains(t, result.Rejeicoes[0].OQue, "avattendant")
}

func TestAPIAnalyzer_AnalyzeSection_Campo(t *testing.T) {
	users := []User{
		{ID: 1, Firstname: "Maria", Lastname: "Santos", Descriptor: "Maria Santos"},
	}

	notifications := []Notification{
		// Declined - should be detected
		{
			ID:             1,
			CongregationID: 48092,
			Date:           "2026-03-01",
			Type:           "fm",
			Status:         "declined",
			Assignee:       1,
			Part:           100,
		},
	}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/fm", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Campo")
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, "Campo", result.Secao)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "Maria Santos", result.Rejeicoes[0].Quem)
}

func TestAPIAnalyzer_AnalyzeSection_TestemunhoPublico(t *testing.T) {
	users := []User{
		{ID: 1, Firstname: "Pedro", Lastname: "Souza", Descriptor: "Pedro Souza"},
	}

	notifications := []Notification{
		// Declined - should be detected
		{
			ID:             1,
			CongregationID: 48092,
			Date:           "2026-03-01",
			Type:           "pubwit",
			Status:         "declined",
			Assignee:       1,
			Part:           200,
		},
	}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/pubwit", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Testemunho Público")
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, "Testemunho Público", result.Secao)
	assert.Equal(t, 1, result.Total)
}

func TestAPIAnalyzer_LoadUsersError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Partes Mecânicas")
	require.NoError(t, err)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to load users")
}

func TestAPIAnalyzer_GetNotificationsError(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test"}}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/ava", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Partes Mecânicas")
	require.NoError(t, err)
	assert.Error(t, result.Error)
}

func TestAPIAnalyzer_AnalyzeAllSections(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test", Descriptor: "Test User"}}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	pathAva := fmt.Sprintf("/scheduling/notifications/%s_%s/ava", startRange, endRange)
	pathFm := fmt.Sprintf("/scheduling/notifications/%s_%s/fm", startRange, endRange)
	pathPubwit := fmt.Sprintf("/scheduling/notifications/%s_%s/pubwit", startRange, endRange)
	pathMm := fmt.Sprintf("/scheduling/notifications/%s_%s/mm", startRange, endRange)
	meetingPath := fmt.Sprintf("/scheduling/mm/meeting/%s_%s", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case pathAva, pathFm, pathPubwit, pathMm:
			json.NewEncoder(w).Encode([]Notification{})
		case meetingPath:
			json.NewEncoder(w).Encode([]Meeting{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	results, err := analyzer.AnalyzeAllSections()
	require.NoError(t, err)
	assert.Len(t, results, 4)
	assert.Equal(t, "Partes Mecânicas", results[0].Secao)
	assert.Equal(t, "Campo", results[1].Secao)
	assert.Equal(t, "Testemunho Público", results[2].Secao)
	assert.Equal(t, "Reunião Meio de Semana", results[3].Secao)
}

func TestAPIAnalyzer_GetUserName(t *testing.T) {
	users := []User{
		{ID: 1, Firstname: "João", Lastname: "Silva", Descriptor: "João Silva"},
		{ID: 2, Firstname: "Maria", Lastname: "Santos"}, // No descriptor
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(UsersResponse{Users: users})
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	// Load users
	analyzer.loadUsers()

	// Test getUserName
	name1 := analyzer.getUserName(1)
	assert.Equal(t, "João Silva", name1)

	name2 := analyzer.getUserName(2)
	assert.Equal(t, "Maria Santos", name2)

	name3 := analyzer.getUserName(999) // Unknown user
	assert.Equal(t, "User 999", name3)
}

func TestAPIAnalyzer_UserCacheReuse(t *testing.T) {
	callCount := 0
	users := []User{{ID: 1, Firstname: "Test"}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fsreport/users" {
			callCount++
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		} else {
			json.NewEncoder(w).Encode([]Notification{})
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	// First call should load users
	analyzer.AnalyzeSection("Partes Mecânicas")
	assert.Equal(t, 1, callCount)

	// Second call should reuse cache
	analyzer.AnalyzeSection("Partes Mecânicas")
	assert.Equal(t, 1, callCount) // Should not increase
}

func TestAPIAnalyzer_EmptyResponses(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test"}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		default:
			json.NewEncoder(w).Encode([]Notification{})
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Partes Mecânicas")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.Empty(t, result.Rejeicoes)
}

func TestAPIAnalyzer_SectionAliases(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test"}}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	pathAva := fmt.Sprintf("/scheduling/notifications/%s_%s/ava", startRange, endRange)
	pathFm := fmt.Sprintf("/scheduling/notifications/%s_%s/fm", startRange, endRange)
	pathPubwit := fmt.Sprintf("/scheduling/notifications/%s_%s/pubwit", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case pathAva, pathFm, pathPubwit:
			json.NewEncoder(w).Encode([]Notification{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	// Test section aliases
	sections := []string{"avattendant", "fsMeeting", "publicWitnessing"}
	for _, section := range sections {
		result, err := analyzer.AnalyzeSection(section)
		require.NoError(t, err)
		require.NoError(t, result.Error)
	}
}

func TestAPIAnalyzer_MultipleDeclined(t *testing.T) {
	users := []User{
		{ID: 1, Firstname: "João", Lastname: "Silva", Descriptor: "João Silva"},
		{ID: 2, Firstname: "Maria", Lastname: "Santos", Descriptor: "Maria Santos"},
	}

	notifications := []Notification{
		// Multiple declined notifications
		{
			ID:             1,
			CongregationID: 48092,
			Date:           "2026-03-01",
			Type:           "pubwit",
			Status:         "declined",
			Assignee:       1,
			Part:           100,
		},
		{
			ID:             2,
			CongregationID: 48092,
			Date:           "2026-03-02",
			Type:           "pubwit",
			Status:         "declined",
			Assignee:       2,
			Part:           101,
		},
		{
			ID:             3,
			CongregationID: 48092,
			Date:           "2026-03-03",
			Type:           "pubwit",
			Status:         "pending", // Not declined
			Assignee:       1,
			Part:           102,
		},
	}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/pubwit", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	result, err := analyzer.AnalyzeSection("Testemunho Público")
	require.NoError(t, err)
	require.NoError(t, result.Error)
	assert.Equal(t, 2, result.Total) // Only 2 declined
	assert.Len(t, result.Rejeicoes, 2)
	assert.Equal(t, "João Silva", result.Rejeicoes[0].Quem)
	assert.Equal(t, "Maria Santos", result.Rejeicoes[1].Quem)
}

func TestAPIAnalyzer_AnalyzeAllSections_WithError(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test"}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		default:
			// Return error for all notification endpoints
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	results, err := analyzer.AnalyzeAllSections()
	require.NoError(t, err)
	assert.Len(t, results, 4)
	// All sections should have errors
	for _, result := range results {
		assert.Error(t, result.Error)
	}
}

func TestAPIAnalyzer_SetDaysToLookAhead(t *testing.T) {
	client := NewClient()
	analyzer := NewAPIAnalyzer(client)

	analyzer.SetDaysToLookAhead(365)
	assert.Equal(t, 365, analyzer.daysToLookAhead)
}

func TestGetMidweekFlagName(t *testing.T) {
	tests := []struct {
		flag     int
		expected string
	}{
		{10, "Leitor do EBC"},
		{11, "Leitor do EBC"},
		{20, "Orador/Dirigente"},
		{30, "Estudante"},
		{40, "Ajudante"},
		{50, "Designação Especial"},
		{60, "Outra Designação"},
		{99, "Designação (flag 99)"},
		{0, "Designação (flag 0)"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("flag_%d", tt.flag), func(t *testing.T) {
			result := getMidweekFlagName(tt.flag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFriendlyTypeName(t *testing.T) {
	tests := []struct {
		typeName string
		expected string
	}{
		{"video", "Vídeo"},
		{"console", "Console"},
		{"mics", "Microfone"},
		{"attendant", "Atendente"},
		{"ava", "Áudio/Vídeo & Indicadores"},
		{"pubwit", "Testemunho Público"},
		{"fm", "Reunião de Campo"},
		{"unknown", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			result := getFriendlyTypeName(tt.typeName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDateToBrazilian(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-03-02", "02/03/2026"},
		{"2024-12-31", "31/12/2024"},
		{"2020-01-01", "01/01/2020"},
		{"invalid", "invalid"},
		{"2026/03/02", "2026/03/02"},
		{"", ""},
		{"2026-03", "2026-03"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := formatDateToBrazilian(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIAnalyzer_AnalyzeMidweekMeetings(t *testing.T) {
	users := []User{{ID: 1, Firstname: "João", Lastname: "Silva", Descriptor: "João Silva"}}

	// Create meetings with parts
	meetings := []Meeting{{
		TGW: []MeetingPart{{ID: 100, Title: "Leitura da Bíblia"}},
		FM:  []MeetingPart{{ID: 101, Title: "Apresentação"}},
		LAC: []MeetingPart{{ID: 102, Title: "Discussão"}},
	}}

	notifications := []Notification{
		{
			ID:             1,
			CongregationID: 48092,
			Date:           "2026-03-01",
			Type:           "mm",
			Status:         "declined",
			Assignee:       1,
			Part:           100,
			Flag:           10,
		},
		{
			ID:             2,
			CongregationID: 48092,
			Date:           "2026-03-02",
			Type:           "mm",
			Status:         "declined",
			Assignee:       1,
			Part:           101,
			Flag:           20,
		},
		{
			ID:             3,
			CongregationID: 48092,
			Date:           "2026-03-03",
			Type:           "mm",
			Status:         "pending",
			Assignee:       1,
			Part:           102,
			Flag:           30,
		},
	}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/mm", startRange, endRange)
	meetingPath := fmt.Sprintf("/scheduling/mm/meeting/%s_%s", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		case meetingPath:
			json.NewEncoder(w).Encode(meetings)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	rejeicoes, err := analyzer.analyzeMidweekMeetings()
	require.NoError(t, err)
	assert.Len(t, rejeicoes, 2)
	assert.Equal(t, "Leitura da Bíblia", rejeicoes[0].OQue)
	assert.Equal(t, "Apresentação", rejeicoes[1].OQue)
}

func TestAPIAnalyzer_AnalyzeMidweekMeetings_GetNotificationsError(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test"}}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/mm", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	_, err := analyzer.analyzeMidweekMeetings()
	assert.Error(t, err)
}

func TestAPIAnalyzer_AnalyzeMidweekMeetings_GetMeetingsError(t *testing.T) {
	users := []User{{ID: 1, Firstname: "Test"}}

	notifications := []Notification{{
		ID:             1,
		CongregationID: 48092,
		Date:           "2026-03-01",
		Type:           "mm",
		Status:         "declined",
		Assignee:       1,
		Part:           100,
		Flag:           10,
	}}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/mm", startRange, endRange)
	meetingPath := fmt.Sprintf("/scheduling/mm/meeting/%s_%s", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		case meetingPath:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	_, err := analyzer.analyzeMidweekMeetings()
	assert.Error(t, err)
}

func TestAPIAnalyzer_AnalyzeMidweekMeetings_FallbackFlagName(t *testing.T) {
	users := []User{{ID: 1, Firstname: "João", Lastname: "Silva", Descriptor: "João Silva"}}

	// Create meetings without the part ID
	meetings := []Meeting{{TGW: []MeetingPart{{ID: 999, Title: "Some Other Part"}}}}

	notifications := []Notification{{
		ID:             1,
		CongregationID: 48092,
		Date:           "2026-03-01",
		Type:           "mm",
		Status:         "declined",
		Assignee:       1,
		Part:           100, // Not in meetings
		Flag:           30,
	}}

	startRange := time.Now().Format("2006-01-02")
	endRange := time.Now().AddDate(0, 0, 730).Format("2006-01-02")
	expectedPath := fmt.Sprintf("/scheduling/notifications/%s_%s/mm", startRange, endRange)
	meetingPath := fmt.Sprintf("/scheduling/mm/meeting/%s_%s", startRange, endRange)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fsreport/users":
			json.NewEncoder(w).Encode(UsersResponse{Users: users})
		case expectedPath:
			json.NewEncoder(w).Encode(notifications)
		case meetingPath:
			json.NewEncoder(w).Encode(meetings)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL
	analyzer := NewAPIAnalyzer(client)

	rejeicoes, err := analyzer.analyzeMidweekMeetings()
	require.NoError(t, err)
	assert.Len(t, rejeicoes, 1)
	assert.Equal(t, "Estudante", rejeicoes[0].OQue)
}
