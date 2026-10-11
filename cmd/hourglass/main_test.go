package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
		{"wm", []string{"schedule", "wm"}},
		{"privileges", []string{"schedule", "privileges"}},
		{"check-rules", []string{"schedule", "check-rules"}},
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{Users: []hourglass.User{{
				ID: 1, Firstname: "Ada", Lastname: "Lovelace", Descriptor: "Ada Lovelace",
			}}})
		case strings.Contains(r.URL.Path, "/scheduling/mm/meeting/"):
			_ = json.NewEncoder(w).Encode([]hourglass.Meeting{{Date: "2026-01-01", LGroup: 1}})
		case strings.Contains(r.URL.Path, "/scheduling/av_attendant/"):
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{{ID: 9, Type: "video", Date: "2026-01-01"}})
		case strings.Contains(r.URL.Path, "/scheduling/notifications/"):
			_ = json.NewEncoder(w).Encode([]hourglass.Notification{{ID: 3, Status: "declined", Type: "pubwit"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"users", []string{"users", "--json"}, "Ada"},
		{"schedule mm", []string{"schedule", "mm", "--from", "2026-01-01", "--to", "2026-01-31", "--lgroup", "1", "--json"}, "2026-01-01"},
		{"schedule av", []string{"schedule", "av", "--from", "2026-01-01", "--to", "2026-01-31", "--json"}, "video"},
		{"schedule notifications", []string{"schedule", "notifications", "--from", "2026-01-01", "--to", "2026-01-31", "--type", "declined", "--json"}, "declined"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cli := newCLI(&stdout, &stderr, os.Getenv)
			cli.client.SetBaseURL(server.URL + "/api/v0.2")
			err := cli.dispatch(tc.args)
			require.NoError(t, err, stderr.String())
			assert.Contains(t, stdout.String(), tc.want)
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
	assert.Error(t, cli.ScheduleNotifications("2026-01-01", "2026-01-31", "declined"))
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
		{"notifications bad type", []string{"schedule", "notifications", "--from", "a", "--to", "b", "--type", "other"}, "invalid notification type"},
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
	assert.Equal(t, 2, code)

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
