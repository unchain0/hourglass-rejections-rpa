package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
	"hourglass-rejections-rpa/src/services/hourglass"
)

var (
	osExit      = os.Exit
	marshalJSON = json.Marshal
)

// CLI is the read-only Hourglass command-line surface.
type CLI struct {
	client       *hourglass.Client
	stdout       io.Writer
	stderr       io.Writer
	getenv       func(string) string
	ensureTokens func() (*webauthn.AuthTokens, error)
}

// NewCLI creates a CLI whose every HTTP request passes through the send blocklist.
func NewCLI() *CLI {
	return newCLI(os.Stdout, os.Stderr, os.Getenv)
}

func newCLI(stdout, stderr io.Writer, getenv func(string) string) *CLI {
	if getenv == nil {
		getenv = os.Getenv
	}
	client := hourglass.NewClient()
	allowSend := getenv("HOURGLASS_ALLOW_SEND") == "1"
	client.SetTransport(&blocklistTransport{
		base:      client.GetTransport(),
		allowSend: allowSend,
	})
	return &CLI{
		client: client,
		stdout: stdout,
		stderr: stderr,
		getenv: getenv,
	}
}

// bootstrapWebAuthn wires credentials/tokens/profile env into the Hourglass client.
// Enables the token manager and loads persisted tokens. Does not start the
// background renewal loop; one-shot CLI calls use EnsureValidTokens on demand.
func (c *CLI) bootstrapWebAuthn() func() {
	credentialsPath := c.getenv("WEBAUTHN_CREDENTIALS_PATH")
	tokensPath := c.getenv("WEBAUTHN_TOKENS_PATH")
	if tokensPath == "" {
		tokensPath = c.getenv("TOKENS_PATH")
	}
	profileDir := c.getenv("CHROME_PROFILE_DIR")
	baseURL := c.getenv("HOURGLASS_URL")

	if baseURL != "" {
		c.client.SetBaseURL(baseURL)
	}
	if tokensPath != "" {
		c.client.SetWebAuthnTokensPath(tokensPath)
	}
	if profileDir != "" {
		c.client.SetBrowserProfileDir(profileDir)
	}

	if credentialsPath == "" && profileDir == "" && tokensPath == "" {
		return nil
	}

	if err := c.client.EnableWebAuthn(credentialsPath, nil); err != nil {
		return nil
	}
	_ = c.client.LoadPersistedAuthTokens()
	return func() { c.client.StopTokenManager() }
}

// EnsureValidTokens reports current WebAuthn session tokens when configured.
func (c *CLI) EnsureValidTokens() (*webauthn.AuthTokens, error) {
	if c.ensureTokens != nil {
		return c.ensureTokens()
	}
	return c.client.EnsureValidTokens()
}

type exitCode2Error struct {
	msg string
}

func (e *exitCode2Error) Error() string {
	return e.msg
}

func exitCode2(msg string) error {
	return &exitCode2Error{msg: msg}
}

// AuthStatus reports credential/token presence without printing secret values.
func (c *CLI) AuthStatus() error {
	credentialsPath := c.getenv("WEBAUTHN_CREDENTIALS_PATH")
	tokensPath := c.getenv("WEBAUTHN_TOKENS_PATH")

	status := map[string]any{
		"webauthn_credentials_path": credentialsPath,
		"tokens_path":               tokensPath,
		"has_credentials":           credentialsPath != "",
		"has_tokens":                tokensPath != "",
	}

	if tokensPath == "" {
		status["tokens_valid"] = false
		status["tokens_error"] = "no tokens path configured"
	} else {
		tokens, err := c.EnsureValidTokens()
		if err != nil {
			status["tokens_valid"] = false
			status["tokens_error"] = err.Error()
		} else {
			status["tokens_valid"] = true
			status["tokens_expires_at"] = tokens.ExpiresAt.Format(time.RFC3339)
			status["hglogin_present"] = tokens.HGLogin != ""
			status["xsrf_present"] = tokens.XSRFToken != ""
		}
	}

	out, err := marshalJSON(status)
	if err != nil {
		return fmt.Errorf("failed to marshal auth status: %w", err)
	}
	fmt.Fprintln(c.stdout, string(out))
	return nil
}

// AuthLogin runs headless passkey authentication when credentials exist.
// It never registers a passkey; missing credentials exit 2 with setup-auth guidance.
func (c *CLI) AuthLogin() error {
	credentialsPath := c.getenv("WEBAUTHN_CREDENTIALS_PATH")
	if credentialsPath == "" {
		return exitCode2("passkey not registered: run setup-auth")
	}
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		return exitCode2("passkey not registered: run setup-auth")
	}
	tokens, err := c.EnsureValidTokens()
	if err != nil {
		return fmt.Errorf("passkey login failed: %w", err)
	}
	if tokens == nil {
		return errors.New("passkey login failed: no tokens returned")
	}
	fmt.Fprintf(c.stdout, "passkey login: ok, tokens valid until %s\n", tokens.ExpiresAt.Format(time.RFC3339))
	return nil
}

// Users prints GET /fsreport/users as JSON.
func (c *CLI) Users() error {
	users, err := c.client.GetUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}
	return c.printJSON(users)
}

// ScheduleMeetings prints GET /scheduling/mm/meeting/{start}_{end} as JSON.
func (c *CLI) ScheduleMeetings(start, end string, lgroup int) error {
	meetings, err := c.client.GetMeetings(start, end, lgroup)
	if err != nil {
		return fmt.Errorf("failed to get meetings: %w", err)
	}
	return c.printJSON(meetings)
}

// ScheduleAVAttendants prints GET /scheduling/av_attendant/{start}_{end} as JSON.
func (c *CLI) ScheduleAVAttendants(start, end string) error {
	attendants, err := c.client.GetAVAttendants(start, end)
	if err != nil {
		return fmt.Errorf("failed to get AV attendants: %w", err)
	}
	return c.printJSON(attendants)
}

// ScheduleNotifications prints GET /scheduling/notifications/{start}_{end}/{type} as JSON.
func (c *CLI) ScheduleNotifications(start, end, notifType string) error {
	notifications, err := c.client.GetNotifications(start, end, notifType)
	if err != nil {
		return fmt.Errorf("failed to get notifications: %w", err)
	}
	return c.printJSON(notifications)
}

func (c *CLI) printJSON(v any) error {
	out, err := marshalJSON(v)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	fmt.Fprintln(c.stdout, string(out))
	return nil
}

// ScheduleWM prints Watchtower Meeting assignments for a date range as JSON.
func (c *CLI) ScheduleWM(start, end string) error {
	meetings, err := c.client.GetWM(start, end)
	if err != nil {
		return fmt.Errorf("failed to get WM: %w", err)
	}
	return c.printJSON(meetings)
}

// Privileges prints privileges/assignments status information as JSON.
func (c *CLI) Privileges() error {
	privileges, err := c.client.GetPrivileges()
	if err != nil {
		return fmt.Errorf("failed to get privileges: %w", err)
	}
	return c.printJSON(privileges)
}

// Assign is not implemented yet; dry-run unless --apply is supplied by the caller.
func (c *CLI) Assign() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// Unassign is not implemented yet; dry-run unless --apply is supplied by the caller.
func (c *CLI) Unassign() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// CheckRules retrieves check rules status information as JSON.
func (c *CLI) CheckRules() error {
	rules, err := c.client.GetCheckRules()
	if err != nil {
		return fmt.Errorf("failed to get check rules: %w", err)
	}
	return c.printJSON(rules)
}

func (c *CLI) dispatch(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(c.stderr, "Usage: hourglass <command> [args...]\n")
		return errors.New("no command provided")
	}

	switch args[0] {
	case "auth":
		return c.runAuth(args[1:])
	case "users":
		return c.runUsers(args[1:])
	case "schedule":
		return c.runSchedule(args[1:])
	default:
		fmt.Fprintf(c.stderr, "Unknown command: %s\n", args[0])
		return errors.New("unknown command")
	}
}

func (c *CLI) runAuth(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(c.stderr, "Usage: hourglass auth <status|login>\n")
		return errors.New("no auth subcommand provided")
	}
	switch args[0] {
	case "status":
		return c.AuthStatus()
	case "login":
		return c.AuthLogin()
	default:
		fmt.Fprintf(c.stderr, "Unknown auth subcommand: %s\n", args[0])
		fmt.Fprintf(c.stderr, "Usage: hourglass auth <status|login>\n")
		return errors.New("unknown auth subcommand")
	}
}

func (c *CLI) runUsers(args []string) error {
	jsonFlag := false
	for _, arg := range args {
		if arg == "--json" {
			jsonFlag = true
		}
	}
	if !jsonFlag {
		fmt.Fprintf(c.stderr, "Usage: hourglass users --json\n")
		return errors.New("missing --json flag")
	}
	return c.Users()
}

type scheduleArgs struct {
	subcommand string
	fromDate   string
	toDate     string
	lgroup     int
	notifType  string
	apply      bool
}

func parseScheduleArgs(args []string) (scheduleArgs, error) {
	var parsed scheduleArgs
	for i := 0; i < len(args); {
		switch args[i] {
		case "wm", "privileges", "assign", "unassign", "check-rules", "mm", "av", "notifications":
			parsed.subcommand = args[i]
			i++
		case "--from":
			val, next, err := takeFlagValue(args, i, "--from")
			if err != nil {
				return parsed, err
			}
			parsed.fromDate = val
			i = next
		case "--to":
			val, next, err := takeFlagValue(args, i, "--to")
			if err != nil {
				return parsed, err
			}
			parsed.toDate = val
			i = next
		case "--lgroup":
			val, next, err := takeFlagValue(args, i, "--lgroup")
			if err != nil {
				return parsed, err
			}
			if _, err := fmt.Sscanf(val, "%d", &parsed.lgroup); err != nil {
				parsed.lgroup = 0
			}
			i = next
		case "--type":
			val, next, err := takeFlagValue(args, i, "--type")
			if err != nil {
				return parsed, err
			}
			parsed.notifType = val
			i = next
		case "--apply":
			parsed.apply = true
			i++
		case "--json":
			i++
		default:
			i++
		}
	}
	return parsed, nil
}

func takeFlagValue(args []string, i int, name string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("missing value for %s", name)
	}
	return args[i+1], i + 2, nil
}

func (c *CLI) runSchedule(args []string) error {
	parsed, err := parseScheduleArgs(args)
	if err != nil {
		return err
	}
	return c.dispatchSchedule(parsed)
}

func (c *CLI) dispatchSchedule(parsed scheduleArgs) error {
	switch parsed.subcommand {
	case "wm":
		if parsed.fromDate == "" || parsed.toDate == "" {
			fmt.Fprintf(c.stderr, "Usage: hourglass schedule wm --from YYYY-MM-DD --to YYYY-MM-DD --json\n")
			return errors.New("missing required --from/--to for wm")
		}
		return c.ScheduleWM(parsed.fromDate, parsed.toDate)
	case "privileges":
		return c.Privileges()
	case "assign":
		return c.runAssignLike("assign", parsed.apply, c.Assign)
	case "unassign":
		return c.runAssignLike("unassign", parsed.apply, c.Unassign)
	case "check-rules":
		return c.CheckRules()
	case "mm":
		return c.runScheduleMM(parsed)
	case "av":
		return c.runScheduleAV(parsed)
	case "notifications":
		return c.runScheduleNotifications(parsed)
	default:
		fmt.Fprintf(c.stderr, "Usage: hourglass schedule <wm|privileges|assign|unassign|check-rules|mm|av|notifications> [flags]\n")
		return errors.New("unknown schedule subcommand")
	}
}

func (c *CLI) runAssignLike(name string, apply bool, fn func() error) error {
	if !apply {
		fmt.Fprintf(c.stderr, "Info: %s running in dry-run mode (use --apply to attempt actual operation)\n", name)
	}
	return fn()
}

func (c *CLI) runScheduleMM(parsed scheduleArgs) error {
	if parsed.fromDate == "" || parsed.toDate == "" {
		fmt.Fprintf(c.stderr, "Usage: hourglass schedule mm --from YYYY-MM-DD --to YYYY-MM-DD [--lgroup N] --json\n")
		return errors.New("missing required flags")
	}
	return c.ScheduleMeetings(parsed.fromDate, parsed.toDate, parsed.lgroup)
}

func (c *CLI) runScheduleAV(parsed scheduleArgs) error {
	if parsed.fromDate == "" || parsed.toDate == "" {
		fmt.Fprintf(c.stderr, "Usage: hourglass schedule av --from YYYY-MM-DD --to YYYY-MM-DD --json\n")
		return errors.New("missing required flags")
	}
	return c.ScheduleAVAttendants(parsed.fromDate, parsed.toDate)
}

func (c *CLI) runScheduleNotifications(parsed scheduleArgs) error {
	if parsed.fromDate == "" || parsed.toDate == "" {
		fmt.Fprintf(c.stderr, "Usage: hourglass schedule notifications --from YYYY-MM-DD --to YYYY-MM-DD [--type <any>] --json\n")
		return errors.New("missing required flags")
	}
	return c.ScheduleNotifications(parsed.fromDate, parsed.toDate, parsed.notifType)
}

// blocklistTransport refuses mutating HTTP methods and send-like paths unless allowSend is set.
type blocklistTransport struct {
	base      http.RoundTripper
	allowSend bool
}

func (t *blocklistTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.allowSend {
		switch req.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			return nil, fmt.Errorf("send operations blocked: %s %s", req.Method, req.URL.Path)
		}
		if isSendLikePath(req.URL.Path) {
			return nil, fmt.Errorf("send operation blocked: %s %s", req.Method, req.URL.Path)
		}
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func isSendLikePath(rawPath string) bool {
	path := strings.ToLower(strings.Trim(rawPath, "/"))
	if path == "" {
		return false
	}

	// Legitimate read endpoint: GET /scheduling/notifications/...
	if path == "scheduling/notifications" || strings.HasPrefix(path, "scheduling/notifications/") {
		return false
	}

	segments := strings.Split(path, "/")
	for i, segment := range segments {
		switch segment {
		case "notify", "send", "publish", "email":
			return true
		case "notification":
			if i+1 < len(segments) && segments[i+1] == "send" {
				return true
			}
		}
	}
	return false
}

// run executes the CLI and returns a process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	cli := newCLI(stdout, stderr, os.Getenv)
	if stop := cli.bootstrapWebAuthn(); stop != nil {
		defer stop()
	}
	if err := cli.dispatch(args); err != nil {
		var code2 *exitCode2Error
		if errors.As(err, &code2) {
			fmt.Fprintf(stderr, "Error: %s\n", code2.msg)
			return 2
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}
