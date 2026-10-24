package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
	"hourglass-rejections-rpa/src/services/hourglass"
)

func TestRun_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Usage:")
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"nope"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Unknown command")
}

func TestExitCode2Error_Error(t *testing.T) {
	err := exitCode2("boom")
	assert.Equal(t, "boom", err.Error())
}

func TestRun_StubsExit2(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"assign dry-run", []string{"schedule", "assign"}},
		{"unassign dry-run", []string{"schedule", "unassign"}},
		{"assign apply", []string{"schedule", "assign", "--apply"}},
		{"unassign apply", []string{"schedule", "unassign", "--apply"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			assert.Equal(t, 2, code)
			assert.Contains(t, stderr.String(), "not implemented yet: endpoint not discovered")
			if strings.Contains(tc.name, "dry-run") {
				assert.Contains(t, stderr.String(), "dry-run")
			}
			if strings.Contains(tc.name, "apply") {
				assert.NotContains(t, stderr.String(), "dry-run")
			}
		})
	}
}

func TestRun_AuthLoginMissingCredentials(t *testing.T) {
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
	var stdout, stderr bytes.Buffer
	code := run([]string{"auth", "login"}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "passkey not registered: run setup-auth")
}

func TestRun_AuthLoginMissingFile(t *testing.T) {
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", t.TempDir()+"/missing.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"auth", "login"}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "passkey not registered: run setup-auth")
}

func TestRun_AuthLoginCredentialsPresent(t *testing.T) {
	path := t.TempDir() + "/creds.json"
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", path)
	t.Setenv("WEBAUTHN_TOKENS_PATH", "")
	t.Setenv("TOKENS_PATH", "")
	t.Setenv("CHROME_PROFILE_DIR", "")

	var stdout bytes.Buffer
	cli := newCLI(&stdout, io.Discard, os.Getenv)
	expiry := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	cli.ensureTokens = func() (*webauthn.AuthTokens, error) {
		return &webauthn.AuthTokens{
			HGLogin:   "present",
			XSRFToken: "present",
			ExpiresAt: expiry,
		}, nil
	}
	require.NoError(t, cli.AuthLogin())
	assert.Contains(t, stdout.String(), "passkey login: ok")
	assert.Contains(t, stdout.String(), expiry.Format(time.RFC3339))
}

func TestAuthLogin_EnsureTokensError(t *testing.T) {
	path := t.TempDir() + "/creds.json"
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", path)

	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	cli.ensureTokens = func() (*webauthn.AuthTokens, error) {
		return nil, errors.New("renew failed")
	}
	err := cli.AuthLogin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "passkey login failed")
}

func TestAuthLogin_NilTokens(t *testing.T) {
	path := t.TempDir() + "/creds.json"
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", path)

	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	cli.ensureTokens = func() (*webauthn.AuthTokens, error) {
		return nil, nil
	}
	err := cli.AuthLogin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tokens returned")
}

func TestBootstrapWebAuthn_LoadsPersistedTokens(t *testing.T) {
	dir := t.TempDir()
	creds := dir + "/creds.json"
	tokensPath := dir + "/tokens.json"
	require.NoError(t, os.WriteFile(creds, []byte(`{}`), 0o600))

	expiry := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	payload, err := json.Marshal(webauthn.AuthTokens{
		HGLogin:   "loaded-login",
		XSRFToken: "loaded-xsrf",
		ExpiresAt: expiry,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokensPath, payload, 0o600))

	getenv := func(key string) string {
		switch key {
		case "WEBAUTHN_CREDENTIALS_PATH":
			return creds
		case "WEBAUTHN_TOKENS_PATH":
			return tokensPath
		case "HOURGLASS_URL":
			return "https://example.test"
		default:
			return ""
		}
	}
	cli := newCLI(io.Discard, io.Discard, getenv)
	stop := cli.bootstrapWebAuthn()
	require.NotNil(t, stop)
	t.Cleanup(stop)

	tokens, err := cli.EnsureValidTokens()
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.Equal(t, "loaded-login", tokens.HGLogin)
	assert.Equal(t, "loaded-xsrf", tokens.XSRFToken)
}

func TestBootstrapWebAuthn_NoConfig(t *testing.T) {
	cli := newCLI(io.Discard, io.Discard, func(string) string { return "" })
	assert.Nil(t, cli.bootstrapWebAuthn())
}

func TestBootstrapWebAuthn_ProfileAndTokensFallback(t *testing.T) {
	dir := t.TempDir()
	profile := dir + "/chrome-profile"
	require.NoError(t, os.MkdirAll(profile, 0o700))
	tokensPath := dir + "/from-tokens-path.json"

	getenv := func(key string) string {
		switch key {
		case "CHROME_PROFILE_DIR":
			return profile
		case "TOKENS_PATH":
			return tokensPath
		case "WEBAUTHN_CREDENTIALS_PATH", "WEBAUTHN_TOKENS_PATH":
			return ""
		default:
			return ""
		}
	}
	cli := newCLI(io.Discard, io.Discard, getenv)
	stop := cli.bootstrapWebAuthn()
	require.NotNil(t, stop)
	stop()
}

func TestBootstrapWebAuthn_EnableFailure(t *testing.T) {
	// Force EnableWebAuthn to fail by making credentials path a file that cannot
	// be used as a parent directory for NewStorage (path is an existing file).
	dir := t.TempDir()
	blocked := dir + "/not-a-dir"
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))
	// storage path = blocked/creds.json -> MkdirAll(blocked) fails because blocked is a file
	creds := blocked + "/creds.json"

	getenv := func(key string) string {
		if key == "WEBAUTHN_CREDENTIALS_PATH" {
			return creds
		}
		return ""
	}
	cli := newCLI(io.Discard, io.Discard, getenv)
	assert.Nil(t, cli.bootstrapWebAuthn())
}

func TestRun_AuthStatusNoSecrets(t *testing.T) {
	secretLogin := "super-secret-hglogin-value"
	secretXSRF := "super-secret-xsrf-value"
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "/tmp/creds")
	t.Setenv("WEBAUTHN_TOKENS_PATH", "/tmp/tokens")

	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	cli.stdout = &bytes.Buffer{}
	cli.ensureTokens = func() (*webauthn.AuthTokens, error) {
		return &webauthn.AuthTokens{
			HGLogin:   secretLogin,
			XSRFToken: secretXSRF,
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil
	}
	require.NoError(t, cli.AuthStatus())
	out := cli.stdout.(*bytes.Buffer).String()
	assert.NotContains(t, out, secretLogin)
	assert.NotContains(t, out, secretXSRF)
	assert.Contains(t, out, `"hglogin_present":true`)
	assert.Contains(t, out, `"xsrf_present":true`)
	assert.Contains(t, out, `"tokens_valid":true`)
}

func TestAuthStatus_NoTokensPath(t *testing.T) {
	t.Setenv("WEBAUTHN_TOKENS_PATH", "")
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
	var stdout, stderr bytes.Buffer
	code := run([]string{"auth", "status"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), `"tokens_valid":false`)
	assert.Contains(t, stdout.String(), "no tokens path configured")
}

func TestAuthStatus_TokensError(t *testing.T) {
	t.Setenv("WEBAUTHN_TOKENS_PATH", "/tmp/tokens")
	cli := newCLI(&bytes.Buffer{}, io.Discard, os.Getenv)
	cli.ensureTokens = func() (*webauthn.AuthTokens, error) {
		return nil, errors.New("token boom")
	}
	require.NoError(t, cli.AuthStatus())
	out := cli.stdout.(*bytes.Buffer).String()
	assert.Contains(t, out, `"tokens_valid":false`)
	assert.Contains(t, out, "token boom")
}

func TestAuthStatus_MarshalError(t *testing.T) {
	orig := marshalJSON
	t.Cleanup(func() { marshalJSON = orig })
	marshalJSON = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }

	cli := newCLI(io.Discard, io.Discard, func(string) string { return "" })
	err := cli.AuthStatus()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

func TestRun_AuthUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	assert.Equal(t, 1, run([]string{"auth"}, &stdout, &stderr))
	assert.Contains(t, stderr.String(), "Usage: hourglass auth")

	stderr.Reset()
	assert.Equal(t, 1, run([]string{"auth", "wat"}, &stdout, &stderr))
	assert.Contains(t, stderr.String(), "Unknown auth subcommand")
}

func TestRun_UsersRequiresJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"users"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Usage: hourglass users --json")
}

func TestReadCommands_HTTPTestServer(t *testing.T) {
	assignee := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{{
				ID: 1, Firstname: "Ada", Lastname: "Lovelace", Descriptor: "Ada Lovelace",
			}}})
		case strings.Contains(r.URL.Path, "/scheduling/mm/meeting/"):
			_ = json.NewEncoder(w).Encode([]hourglass.Meeting{{
				Date: "2026-01-01", LGroup: 1,
				TGW: []hourglass.MeetingPart{{ID: 100, Title: "Bible Reading"}},
			}})
		case strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{{
				ID: 9, Type: "video", Date: "2026-01-01", Status: "Aceito", Assignee: &assignee,
			}})
		case strings.Contains(r.URL.Path, "/scheduling/wm/"):
			_ = json.NewEncoder(w).Encode([]hourglass.WMAssignment{{
				ID: 7, Role: "chairman", Date: "2026-01-04", Status: "Recusado", Assignee: &assignee,
			}})
		case strings.HasSuffix(r.URL.Path, "/privileges"):
			_ = json.NewEncoder(w).Encode([]hourglass.PrivilegeAssignment{{
				ID: 4, Type: "reader", Date: "2026-01-04", Status: "Esperando", Assignee: &assignee,
			}})
		case strings.HasSuffix(r.URL.Path, "/check-rules"):
			_ = json.NewEncoder(w).Encode([]hourglass.CheckRule{{
				ID: 2, Rule: "no-double-book", Status: "pending", Part: 100, Assignment: "Bible Reading",
			}})
		case strings.Contains(r.URL.Path, "/scheduling/notifications/"):
			switch {
			case strings.HasSuffix(r.URL.Path, "/mm"):
				_ = json.NewEncoder(w).Encode([]hourglass.Notification{{
					ID: 11, Status: "declined", Type: "mm", Date: "2026-01-01", Part: 100, Assignee: 1,
				}, {
					ID: 12, Status: "pending", Type: "mm", Date: "2026-01-01", Part: 100, Assignee: 2,
				}})
			case strings.HasSuffix(r.URL.Path, "/pubwit"):
				_ = json.NewEncoder(w).Encode([]hourglass.Notification{{
					ID: 3, Status: "declined", Type: "pubwit", Date: "2026-01-02", Assignee: 1, Part: 50,
				}, {
					ID: 4, Status: "complete", Type: "pubwit", Date: "2026-01-02", Assignee: 99, Part: 50,
				}})
			default:
				_ = json.NewEncoder(w).Encode([]hourglass.Notification{})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"users", []string{"users", "--json"}, []string{"Ada"}},
		{"schedule mm", []string{"schedule", "mm", "--from", "2026-01-01", "--to", "2026-01-31", "--lgroup", "1", "--json"}, []string{"2026-01-01", "Bible Reading", "declined", "Ada Lovelace"}},
		{"schedule av", []string{"schedule", "av", "--from", "2026-01-01", "--to", "2026-01-31", "--json"}, []string{"video", "Aceito", "Ada Lovelace"}},
		{"schedule notifications", []string{"schedule", "notifications", "--from", "2026-01-01", "--to", "2026-01-31", "--type", "pubwit", "--json"}, []string{"pubwit", "Ada Lovelace", "\"substitute_assigned\":true"}},
		{"schedule notifications all types", []string{"schedule", "notifications", "--from", "2026-01-01", "--to", "2026-01-31", "--json"}, []string{"declined", "assignee_name"}},
		{"schedule wm", []string{"schedule", "wm", "--from", "2026-01-01", "--to", "2026-01-31", "--json"}, []string{"chairman", "Recusado", "Ada Lovelace"}},
		{"schedule privileges", []string{"schedule", "privileges", "--json"}, []string{"reader", "Esperando", "Ada Lovelace"}},
		{"schedule check-rules", []string{"schedule", "check-rules", "--json"}, []string{"no-double-book", "pending"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cli := newCLI(&stdout, &stderr, os.Getenv)
			cli.client.SetBaseURL(server.URL + "/api/v0.2")
			err := cli.dispatch(tc.args)
			require.NoError(t, err, stderr.String())
			for _, want := range tc.want {
				assert.Contains(t, stdout.String(), want)
			}
		})
	}
}

func TestReadCommands_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	cli.client.SetBaseURL(server.URL + "/api/v0.2")

	assert.Error(t, cli.Users())
	assert.Error(t, cli.ScheduleMeetings("2026-01-01", "2026-01-31", 0))
	assert.Error(t, cli.ScheduleAVAttendants("2026-01-01", "2026-01-31"))
	assert.Error(t, cli.ScheduleNotifications("2026-01-01", "2026-01-31", "mm"))
	assert.Error(t, cli.ScheduleNotifications("2026-01-01", "2026-01-31", ""))
	assert.Error(t, cli.ScheduleWM("2026-01-01", "2026-01-31"))
	assert.Error(t, cli.Privileges())
	assert.Error(t, cli.CheckRules())
}

func TestReadCommands_DownstreamErrors(t *testing.T) {
	assignee := 1
	mode := "users-fail"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/fsreport/users"):
			if mode == "users-fail" {
				http.Error(w, "users down", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{{ID: 1, Descriptor: "Ada"}}})
		case strings.Contains(r.URL.Path, "/scheduling/mm/meeting/"):
			_ = json.NewEncoder(w).Encode([]hourglass.Meeting{{Date: "2026-01-01"}})
		case strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{{ID: 1, Type: "video", Date: "2026-01-01"}})
		case strings.Contains(r.URL.Path, "/scheduling/wm/"):
			_ = json.NewEncoder(w).Encode([]hourglass.WMAssignment{{ID: 1, Role: "reader", Assignee: &assignee}})
		case strings.HasSuffix(r.URL.Path, "/privileges"):
			_ = json.NewEncoder(w).Encode([]hourglass.PrivilegeAssignment{{ID: 1, Type: "reader", Assignee: &assignee}})
		case strings.Contains(r.URL.Path, "/scheduling/notifications/"):
			if mode == "mm-status-fail" && strings.HasSuffix(r.URL.Path, "/mm") {
				http.Error(w, "mm status down", http.StatusInternalServerError)
				return
			}
			notifType := "mm"
			switch {
			case strings.HasSuffix(r.URL.Path, "/ava"):
				notifType = "ava"
			case strings.HasSuffix(r.URL.Path, "/fm"):
				notifType = "fm"
			case strings.HasSuffix(r.URL.Path, "/pubwit"):
				notifType = "pubwit"
			}
			_ = json.NewEncoder(w).Encode([]hourglass.Notification{{
				ID: 1, Type: notifType, Status: "declined", Date: "2026-01-01", Part: 1, Assignee: 1,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	cli.client.SetBaseURL(server.URL + "/api/v0.2")

	mode = "users-fail"
	assert.Error(t, cli.ScheduleAVAttendants("2026-01-01", "2026-01-31"))
	assert.Error(t, cli.ScheduleWM("2026-01-01", "2026-01-31"))
	assert.Error(t, cli.Privileges())
	assert.Error(t, cli.ScheduleNotifications("2026-01-01", "2026-01-31", "ava"))
	assert.Error(t, cli.ScheduleMeetings("2026-01-01", "2026-01-31", 0))

	mode = "mm-status-fail"
	assert.Error(t, cli.ScheduleMeetings("2026-01-01", "2026-01-31", 0))

	// Non-mm type still exercises needsMMTitles false path with successful enrichment.
	mode = "ok"
	require.NoError(t, cli.ScheduleNotifications("2026-01-01", "2026-01-31", "ava"))
}

func TestNeedsMMTitles(t *testing.T) {
	assert.True(t, needsMMTitles("", nil))
	assert.True(t, needsMMTitles("mm", nil))
	assert.False(t, needsMMTitles("ava", nil))
	assert.False(t, needsMMTitles("ava", []hourglass.Notification{{Type: "ava"}}))
	assert.True(t, needsMMTitles("custom", []hourglass.Notification{{Type: "mm"}}))
}

func TestPrintJSON_MarshalError(t *testing.T) {
	orig := marshalJSON
	t.Cleanup(func() { marshalJSON = orig })
	marshalJSON = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }

	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	err := cli.printJSON(map[string]string{"a": "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

func TestRun_ScheduleValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"mm missing flags", []string{"schedule", "mm", "--json"}, "missing required flags"},
		{"av missing flags", []string{"schedule", "av"}, "missing required flags"},
		{"notifications missing flags", []string{"schedule", "notifications", "--from", "2026-01-01"}, "missing required flags"},
		{"notifications bad type", []string{"schedule", "notifications", "--from", "a", "--to", "b", "--type", "accepted"}, "failed to get notifications"},
		{"wm missing flags", []string{"schedule", "wm", "--json"}, "missing required --from/--to for wm"},
		{"no subcommand", []string{"schedule"}, "unknown schedule subcommand"},
		{"missing from value", []string{"schedule", "mm", "--from"}, "missing value for --from"},
		{"missing to value", []string{"schedule", "mm", "--to"}, "missing value for --to"},
		{"missing lgroup value", []string{"schedule", "mm", "--lgroup"}, "missing value for --lgroup"},
		{"missing type value", []string{"schedule", "notifications", "--type"}, "missing value for --type"},
		{"bad lgroup", []string{"schedule", "mm", "--from", "2026-01-01", "--to", "2026-01-02", "--lgroup", "x", "--json"}, "failed to get meetings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			assert.NotEqual(t, 0, code)
			combined := stderr.String() + stdout.String()
			assert.Contains(t, combined, tc.want)
		})
	}
}

func TestNewCLI_WiresBlocklist(t *testing.T) {
	t.Setenv("HOURGLASS_ALLOW_SEND", "")
	cli := NewCLI()
	_, ok := cli.client.GetTransport().(*blocklistTransport)
	assert.True(t, ok, "NewCLI must wrap client transport with blocklistTransport")
}

func TestBlocklistTransport(t *testing.T) {
	recording := &recordingTransport{}

	cases := []struct {
		name      string
		allowSend bool
		method    string
		path      string
		wantErr   bool
	}{
		{"GET users allowed", false, http.MethodGet, "/api/v0.2/fsreport/users", false},
		{"GET notifications allowed", false, http.MethodGet, "/api/v0.2/scheduling/notifications/2026-01-01_2026-01-31/declined", false},
		{"GET notifications root allowed", false, http.MethodGet, "/scheduling/notifications", false},
		{"POST refused", false, http.MethodPost, "/api/v0.2/fsreport/users", true},
		{"PUT refused", false, http.MethodPut, "/api/v0.2/fsreport/users", true},
		{"PATCH refused", false, http.MethodPatch, "/api/v0.2/thing", true},
		{"DELETE refused", false, http.MethodDelete, "/api/v0.2/thing", true},
		{"GET notify refused", false, http.MethodGet, "/api/v0.2/scheduling/notify", true},
		{"GET notification/send refused", false, http.MethodGet, "/api/v0.2/notification/send", true},
		{"GET send refused", false, http.MethodGet, "/api/v0.2/messages/send", true},
		{"GET publish refused", false, http.MethodGet, "/api/v0.2/publish", true},
		{"GET email refused", false, http.MethodGet, "/api/v0.2/email", true},
		{"POST allowed when flag set", true, http.MethodPost, "/api/v0.2/messages/send", false},
		{"GET send allowed when flag set", true, http.MethodGet, "/api/v0.2/messages/send", false},
		{"empty path allowed", false, http.MethodGet, "/", false},
		{"notification without send allowed", false, http.MethodGet, "/api/v0.2/notification/list", false},
		{"PUT av assignee refused without allowAssign", false, http.MethodPut, "/api/v0.2/scheduling/av_attendant/42", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recording.called = false
			rt := &blocklistTransport{base: recording, allowSend: tc.allowSend}
			req := httptest.NewRequest(tc.method, "http://example.test"+tc.path, nil)
			resp, err := rt.RoundTrip(req)
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
				assert.False(t, recording.called)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.True(t, recording.called)
			_ = resp.Body.Close()
		})
	}
}

func TestBlocklistTransport_AllowAssignAssigneeOnly(t *testing.T) {
	recording := &recordingTransport{}
	rt := &blocklistTransport{base: recording, allowAssign: true}

	allowed := httptest.NewRequest(http.MethodPut, "http://example.test/api/v0.2/scheduling/av_attendant/42?lgroup=0", nil)
	resp, err := rt.RoundTrip(allowed)
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()

	recording.called = false
	blockedPublish := httptest.NewRequest(http.MethodPut, "http://example.test/api/v0.2/scheduling/av_attendant/42?publish=1", nil)
	_, err = rt.RoundTrip(blockedPublish)
	require.Error(t, err)
	assert.False(t, recording.called)

	recording.called = false
	blockedNotify := httptest.NewRequest(http.MethodGet, "http://example.test/api/v0.2/scheduling/notify", nil)
	_, err = rt.RoundTrip(blockedNotify)
	require.Error(t, err)

	recording.called = false
	blockedPost := httptest.NewRequest(http.MethodPost, "http://example.test/api/v0.2/scheduling/av_attendant", nil)
	_, err = rt.RoundTrip(blockedPost)
	require.Error(t, err)
	assert.False(t, recording.called)
}

func TestBlocklistTransport_NilBaseUsesDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	rt := &blocklistTransport{base: nil, allowSend: false}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/ok", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestBlocklist_AllowSendEnvReadOnce(t *testing.T) {
	t.Setenv("HOURGLASS_ALLOW_SEND", "1")
	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	bl, ok := cli.client.GetTransport().(*blocklistTransport)
	require.True(t, ok)
	assert.True(t, bl.allowSend)

	t.Setenv("HOURGLASS_ALLOW_SEND", "0")
	cli2 := newCLI(io.Discard, io.Discard, os.Getenv)
	bl2, ok := cli2.client.GetTransport().(*blocklistTransport)
	require.True(t, ok)
	assert.False(t, bl2.allowSend)

	// Mutating env after construction must not change the transport flag.
	t.Setenv("HOURGLASS_ALLOW_SEND", "1")
	assert.False(t, bl2.allowSend)
}

func TestIsSendLikePath(t *testing.T) {
	assert.False(t, isSendLikePath(""))
	assert.False(t, isSendLikePath("/scheduling/notifications/x"))
	assert.True(t, isSendLikePath("/foo/notify/bar"))
	assert.True(t, isSendLikePath("/notification/send"))
	assert.False(t, isSendLikePath("/notification/view"))
}

func TestEnsureValidTokens_DefaultAndOverride(t *testing.T) {
	cli := newCLI(io.Discard, io.Discard, os.Getenv)
	tokens, err := cli.EnsureValidTokens()
	assert.Nil(t, tokens)
	assert.Error(t, err)

	cli.ensureTokens = func() (*webauthn.AuthTokens, error) {
		return &webauthn.AuthTokens{HGLogin: "x", ExpiresAt: time.Now()}, nil
	}
	tokens, err = cli.EnsureValidTokens()
	require.NoError(t, err)
	assert.Equal(t, "x", tokens.HGLogin)
}

func TestMain_SuccessAndErrors(t *testing.T) {
	origArgs := os.Args
	origExit := osExit
	t.Cleanup(func() {
		os.Args = origArgs
		osExit = origExit
	})

	var code int
	osExit = func(c int) { code = c }

	os.Args = []string{"hourglass"}
	main()
	assert.Equal(t, 1, code)

	os.Args = []string{"hourglass", "schedule", "wm"}
	main()
	assert.Equal(t, 1, code) // missing --from/--to

	t.Setenv("WEBAUTHN_TOKENS_PATH", "")
	t.Setenv("WEBAUTHN_CREDENTIALS_PATH", "")
	os.Args = []string{"hourglass", "auth", "status"}
	main()
	assert.Equal(t, 0, code)
}

func TestNewCLI_NilGetenv(t *testing.T) {
	cli := newCLI(io.Discard, io.Discard, nil)
	require.NotNil(t, cli.getenv)
}

func TestSchedule_UnknownGarbageIgnored(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"schedule", "mm", "extra", "--from", "2026-01-01", "--to", "2026-01-02", "--json"}, &stdout, &stderr)
	// Will try real network or fail getting meetings — either way non-zero without server.
	assert.NotEqual(t, 0, code)
}

type recordingTransport struct {
	called bool
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.called = true
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestParseAssignSwapArgs(t *testing.T) {
	opts, err := parseAssignSwapArgs([]string{
		"--slot", "42", "--to", "20", "--from", "2026-01-01", "--until", "2026-01-31",
		"--lgroup", "2", "--dry-run", "--json",
	})
	require.NoError(t, err)
	assert.Equal(t, 42, opts.slotID)
	assert.Equal(t, "20", opts.toUser)
	assert.Equal(t, "2026-01-01", opts.fromDate)
	assert.Equal(t, "2026-01-31", opts.untilDate)
	assert.Equal(t, 2, opts.lgroup)
	assert.True(t, opts.dryRun)

	// Neither dry-run nor confirm defaults to dry-run.
	opts, err = parseAssignSwapArgs([]string{"--slot", "1", "--to", "2"})
	require.NoError(t, err)
	assert.True(t, opts.dryRun)

	_, err = parseAssignSwapArgs([]string{"--slot", "1", "--to", "2", "--dry-run", "--confirm", "abc"})
	assert.ErrorContains(t, err, "either --dry-run or --confirm")

	_, err = parseAssignSwapArgs([]string{"--slot", "nope"})
	assert.ErrorContains(t, err, "invalid --slot")

	_, err = parseAssignSwapArgs([]string{"--slot"})
	assert.ErrorContains(t, err, "missing value")

	_, err = parseAssignSwapArgs([]string{"--unknown"})
	assert.ErrorContains(t, err, "unknown flag")

	_, err = parseAssignSwapArgs([]string{"--lgroup", "x"})
	assert.ErrorContains(t, err, "invalid --lgroup")
}

func TestRunAssign_Usage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"assign"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Usage:")

	code = run([]string{"assign", "nope"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Unknown assign")
}

func TestAssignSwap_GuardRailsAndSuccess(t *testing.T) {
	cur := 10
	next := 20
	updatedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	var putCount int
	var methods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{
				{ID: 10, Descriptor: "Ada Lovelace"},
				{ID: 20, Descriptor: "Bob Builder"},
			}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{{
				ID: 42, Type: "mics", Slot: 1, Date: "2026-03-01",
				Assignee: &cur, UpdatedAt: updatedAt,
			}})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/scheduling/av_attendant/42"):
			putCount++
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `"assignee":20`)
			assert.NotContains(t, string(body), "published")
			assert.NotContains(t, string(body), "notify")
			_ = json.NewEncoder(w).Encode(hourglass.AVAttendant{
				ID: 42, Type: "mics", Slot: 1, Date: "2026-03-01",
				Assignee: &next, UpdatedAt: updatedAt.Add(time.Minute),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL + "/api/v0.2"
	mkCLI := func(allowAssign string) (*CLI, *bytes.Buffer, *bytes.Buffer) {
		var stdout, stderr bytes.Buffer
		cli := newCLI(&stdout, &stderr, func(k string) string {
			switch k {
			case "HOURGLASS_ALLOW_ASSIGN":
				return allowAssign
			case "HOURGLASS_ALLOW_SEND":
				return ""
			default:
				return ""
			}
		})
		cli.client.SetBaseURL(baseURL)
		// Re-wrap transport after SetBaseURL so blocklist still sees env flags.
		bl, _ := cli.client.GetTransport().(*blocklistTransport)
		if bl != nil {
			bl.allowAssign = allowAssign == "1"
			bl.allowSend = false
		}
		return cli, &stdout, &stderr
	}

	argsBase := []string{
		"--slot", "42", "--to", "20",
		"--from", "2026-03-01", "--until", "2026-03-07",
	}

	// Flag missing on apply path (confirm without allow).
	cli, _, _ := mkCLI("")
	err := cli.AssignSwap(assignSwapOpts{slotID: 42, toUser: "20", confirm: "deadbeef", fromDate: "2026-03-01", untilDate: "2026-03-07"})
	var code2 *exitCode2Error
	require.ErrorAs(t, err, &code2)
	assert.Contains(t, err.Error(), "HOURGLASS_ALLOW_ASSIGN")
	assert.Equal(t, 0, putCount)

	// Confirm missing when allow set and not dry-run.
	cli, _, _ = mkCLI("1")
	err = cli.AssignSwap(assignSwapOpts{slotID: 42, toUser: "20", dryRun: false, confirm: "", fromDate: "2026-03-01", untilDate: "2026-03-07"})
	require.ErrorAs(t, err, &code2)
	assert.Contains(t, err.Error(), "missing --confirm")

	// Dry-run: no write, shows names + confirm.
	cli, stdout, _ := mkCLI("")
	err = cli.AssignSwap(assignSwapOpts{slotID: 42, toUser: "20", dryRun: true, fromDate: "2026-03-01", untilDate: "2026-03-07"})
	require.NoError(t, err)
	assert.Equal(t, 0, putCount)
	assert.Contains(t, stdout.String(), "Ada Lovelace")
	assert.Contains(t, stdout.String(), "Bob Builder")
	assert.Contains(t, stdout.String(), `"dry_run":true`)
	var preview hourglass.AssignSwapResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &preview))
	require.NotEmpty(t, preview.Confirm)

	// Confirm mismatch.
	cli, _, _ = mkCLI("1")
	err = cli.AssignSwap(assignSwapOpts{
		slotID: 42, toUser: "20", dryRun: false, confirm: "wrong-token",
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	})
	require.ErrorAs(t, err, &code2)
	assert.Contains(t, err.Error(), "confirm token mismatch")
	assert.Equal(t, 0, putCount)

	// Stale preview (version change).
	stale := hourglass.AssignSwapConfirmToken(42, 10, 20, "other-version")
	err = cli.AssignSwap(assignSwapOpts{
		slotID: 42, toUser: "20", dryRun: false, confirm: stale,
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	})
	require.ErrorAs(t, err, &code2)
	assert.Equal(t, 0, putCount)

	// Unknown slot.
	err = cli.AssignSwap(assignSwapOpts{
		slotID: 999, toUser: "20", dryRun: true,
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	})
	assert.ErrorContains(t, err, "unknown slot")

	// Unknown user.
	err = cli.AssignSwap(assignSwapOpts{
		slotID: 42, toUser: "999", dryRun: true,
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	})
	assert.ErrorContains(t, err, "unknown user")

	// Success: exactly one assignee-only write; no publish/notify.
	methods = nil
	cli, stdout, _ = mkCLI("1")
	err = cli.AssignSwap(assignSwapOpts{
		slotID: 42, toUser: "20", dryRun: false, confirm: preview.Confirm,
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, putCount)
	assert.Contains(t, stdout.String(), `"applied":true`)
	assert.Contains(t, stdout.String(), "Bob Builder")
	assert.NotContains(t, stdout.String(), `"confirm"`)
	for _, m := range methods {
		lower := strings.ToLower(m)
		assert.NotContains(t, lower, "publish")
		assert.NotContains(t, lower, "notify")
		assert.NotContains(t, lower, "/send")
	}

	// Dispatch path dry-run default.
	cli, stdout, _ = mkCLI("")
	err = cli.dispatch(append([]string{"assign", "swap"}, argsBase...))
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"dry_run":true`)

	// Missing required flags.
	err = cli.AssignSwap(assignSwapOpts{dryRun: true})
	assert.ErrorContains(t, err, "missing required --slot")
	err = cli.AssignSwap(assignSwapOpts{slotID: 1, dryRun: true})
	assert.ErrorContains(t, err, "missing required --to")
}

func TestIsAssigneeOnlyWrite(t *testing.T) {
	ok := httptest.NewRequest(http.MethodPut, "http://x/api/v0.2/scheduling/av_attendant/9", nil)
	assert.True(t, isAssigneeOnlyWrite(ok))
	badMethod := httptest.NewRequest(http.MethodPost, "http://x/api/v0.2/scheduling/av_attendant/9", nil)
	assert.False(t, isAssigneeOnlyWrite(badMethod))
	badPath := httptest.NewRequest(http.MethodPut, "http://x/api/v0.2/scheduling/wm/9", nil)
	assert.False(t, isAssigneeOnlyWrite(badPath))
	badQuery := httptest.NewRequest(http.MethodPut, "http://x/api/v0.2/scheduling/av_attendant/9?notify=1", nil)
	assert.False(t, isAssigneeOnlyWrite(badQuery))
	sendQuery := httptest.NewRequest(http.MethodPut, "http://x/api/v0.2/scheduling/av_attendant/9?send=1", nil)
	assert.False(t, isAssigneeOnlyWrite(sendQuery))
}

func TestAssignSwap_DefaultRangeAndSaveFailure(t *testing.T) {
	cur := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{{ID: 2, Descriptor: "Bob"}}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{{
				ID: 5, Type: "stage", Slot: 0, Date: "2026-03-01", Assignee: &cur,
			}})
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`fail`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	cli := newCLI(&stdout, io.Discard, func(k string) string {
		if k == "HOURGLASS_ALLOW_ASSIGN" {
			return "1"
		}
		return ""
	})
	cli.client.SetBaseURL(server.URL + "/api/v0.2")
	bl, _ := cli.client.GetTransport().(*blocklistTransport)
	bl.allowAssign = true

	// Default range path (empty from/until).
	err := cli.AssignSwap(assignSwapOpts{slotID: 5, toUser: "2", dryRun: true})
	require.NoError(t, err)
	var preview hourglass.AssignSwapResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &preview))
	require.NotEmpty(t, preview.Confirm)

	stdout.Reset()
	err = cli.AssignSwap(assignSwapOpts{
		slotID: 5, toUser: "2", dryRun: false, confirm: preview.Confirm,
	})
	assert.ErrorContains(t, err, "failed to swap assignee")
}

func TestNewCLI_AllowAssignEnv(t *testing.T) {
	cli := newCLI(io.Discard, io.Discard, func(k string) string {
		if k == "HOURGLASS_ALLOW_ASSIGN" {
			return "1"
		}
		return ""
	})
	bl, ok := cli.client.GetTransport().(*blocklistTransport)
	require.True(t, ok)
	assert.True(t, bl.allowAssign)
	assert.False(t, bl.allowSend)
}

func TestAssignSwap_RefetchRejectsConcurrentEdit(t *testing.T) {
	cur := 10
	updatedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{
				{ID: 10, Descriptor: "Ada"}, {ID: 20, Descriptor: "Bob"},
			}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			gets++
			row := hourglass.AVAttendant{
				ID: 42, Type: "mics", Slot: 1, Date: "2026-03-01",
				Assignee: &cur, UpdatedAt: updatedAt,
			}
			if gets > 1 {
				// Concurrent edit bumped version after the dry-run snapshot.
				row.UpdatedAt = updatedAt.Add(time.Hour)
			}
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{row})
		case r.Method == http.MethodPut:
			t.Fatal("PUT must not run when confirm is stale after re-fetch")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	cli := newCLI(&stdout, io.Discard, func(k string) string {
		if k == "HOURGLASS_ALLOW_ASSIGN" {
			return "1"
		}
		return ""
	})
	cli.client.SetBaseURL(server.URL + "/api/v0.2")
	bl, _ := cli.client.GetTransport().(*blocklistTransport)
	bl.allowAssign = true

	require.NoError(t, cli.AssignSwap(assignSwapOpts{
		slotID: 42, toUser: "20", dryRun: true,
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	}))
	var preview hourglass.AssignSwapResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &preview))

	err := cli.AssignSwap(assignSwapOpts{
		slotID: 42, toUser: "20", dryRun: false, confirm: preview.Confirm,
		fromDate: "2026-03-01", untilDate: "2026-03-07",
	})
	var code2 *exitCode2Error
	require.ErrorAs(t, err, &code2)
	assert.Contains(t, err.Error(), "stale preview")
}

func TestCleanupHistory_FiltersTypesWithoutWrites(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{
			{ID: 1, Date: "2026-03-01", Type: "mics"},
			{ID: 2, Date: "2026-03-01", Type: "video"},
		})
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	cli := newCLI(&stdout, io.Discard, func(string) string { return "" })
	cli.client.SetBaseURL(server.URL + "/api/v0.2")
	require.NoError(t, cli.dispatch([]string{"cleanup", "history", "--from", "2026-03-01", "--to", "2026-03-31", "--types", "mics", "--json"}))
	assert.Contains(t, stdout.String(), `"id":1`)
	assert.NotContains(t, stdout.String(), `"id":2`)
	assert.Equal(t, []string{"GET /api/v0.2/scheduling/av_attendant/2026-03-01_2026-03-31"}, methods)
}

func TestCleanupPlan_DryRunAndApplyGuards(t *testing.T) {
	updatedAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	var putCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(r.URL.Path, "/fsreport/users") {
				_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{{ID: 7, Descriptor: "Ada"}}})
				return
			}
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{{ID: 42, Date: "2026-03-01", Type: "mics", UpdatedAt: updatedAt}})
		case http.MethodPut:
			putCount++
			_ = json.NewEncoder(w).Encode(hourglass.AVAttendant{ID: 42, Date: "2026-03-01", Type: "mics", Assignee: intPtr(7), UpdatedAt: updatedAt})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	rules := filepath.Join(t.TempDir(), "rules.json")
	require.NoError(t, os.WriteFile(rules, []byte(`{"eligible_user_ids":[7],"types":["mics"],"max_assignments_per_user_per_month":1}`), 0o600))

	var stdout bytes.Buffer
	cli := newCLI(&stdout, io.Discard, func(string) string { return "" })
	cli.client.SetBaseURL(server.URL + "/api/v0.2")
	require.NoError(t, cli.dispatch([]string{"cleanup", "plan", "--month", "2026-03", "--rules", rules, "--history-months", "1", "--types", "mics", "--json"}))
	assert.Equal(t, 0, putCount)
	var preview hourglass.CleanupPlan
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &preview))
	require.Len(t, preview.Mutations, 1)
	assert.True(t, preview.DryRun)

	stdout.Reset()
	err := cli.dispatch([]string{"cleanup", "plan", "--month", "2026-03", "--rules", rules, "--types", "mics", "--apply", "--confirm", preview.Confirm, "--json"})
	var code2 *exitCode2Error
	require.ErrorAs(t, err, &code2)
	assert.Equal(t, 0, putCount)
}

func intPtr(v int) *int { return &v }

func TestAuthBootstrapFirefox_ExtractsCookiesAtomically(t *testing.T) {
	profile := t.TempDir()
	dbPath := filepath.Join(profile, "cookies.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE moz_cookies (host TEXT, name TEXT, value TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO moz_cookies(host, name, value) VALUES (?, ?, ?), (?, ?, ?)`, "app.hourglass-app.com", "hglogin", "test-hg", "app.hourglass-app.com", "X-Hourglass-XSRF-Token", "test-xsrf")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	out := filepath.Join(t.TempDir(), "private", "tokens.json")
	var stdout bytes.Buffer
	cli := newCLI(&stdout, io.Discard, func(string) string { return "" })
	require.NoError(t, cli.dispatch([]string{"auth", "bootstrap-firefox", "--profile", profile, "--tokens-out", out}))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	var tokens webauthn.AuthTokens
	require.NoError(t, json.Unmarshal(data, &tokens))
	assert.Equal(t, "test-hg", tokens.HGLogin)
	assert.Equal(t, "test-xsrf", tokens.XSRFToken)
	assert.NotContains(t, stdout.String(), "test-hg")
	assert.NotContains(t, stdout.String(), "test-xsrf")
	info, err := os.Stat(out)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	parent, err := os.Stat(filepath.Dir(out))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), parent.Mode().Perm())
}
