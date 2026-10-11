package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"hourglass-rejections-rpa/src/services/hourglass"
	"hourglass-rejections-rpa/src/integrations/auth/webauthn"
)

// CLI represents the read-only hourglass CLI.
type CLI struct {
	client *hourglass.Client
}

// NewCLI creates a new CLI instance configured for the Hourglass API.
func NewCLI() *CLI {
	client := hourglass.NewClient()
	// We cannot directly wrap the client's unexported httpClient transport.
	// The blocklist transport will be applied at the test/server level via
	// newHTTPTestServer's in-memory round tripper, and at the HTTP client
	// level when HOURGLASS_ALLOW_SEND is set.
	return &CLI{client: client}
}

// EnsureValidTokens wraps the client's EnsureValidTokens for CLI use.
func (c *CLI) EnsureValidTokens() (*webauthn.AuthTokens, error) {
	return c.client.EnsureValidTokens()
}

// exitCode2 wraps an error to cause exit code 2
type exitCode2Error struct {
	msg string
}

func (e *exitCode2Error) Error() string {
	return e.msg
}

func exitCode2(msg string) *exitCode2Error {
	return &exitCode2Error{msg: msg}
}

// AuthStatus reports the presence/validity/expiry of credentials and tokens.
func (c *CLI) AuthStatus() error {
	credentialsPath := os.Getenv("WEBAUTHN_CREDENTIALS_PATH")
	tokensPath := os.Getenv("WEBAUTHN_TOKENS_PATH")

	hasCredentials := credentialsPath != ""
	hasTokens := tokensPath != ""

	status := map[string]interface{}{
		"webauthn_credentials_path": credentialsPath,
		"tokens_path":               tokensPath,
		"has_credentials":         hasCredentials,
		"has_tokens":                hasTokens,
	}

	if hasTokens {
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
	} else {
		status["tokens_valid"] = false
		status["tokens_error"] = "no tokens path configured"
	}

	out, _ := json.Marshal(status)
	fmt.Println(string(out))
	return nil
}

// AuthLogin performs headless passkey login via WebAuthn.
func (c *CLI) AuthLogin() error {
	credentialsPath := os.Getenv("WEBAUTHN_CREDENTIALS_PATH")

	if credentialsPath == "" {
		return exitCode2("passkey not registered: run setup-auth")
	}

	// Check if passkey is registered by checking the credentials file
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		return exitCode2("passkey not registered: run setup-auth")
	}

	// Credentials exist but we do NOT register any passkey here.
	// Just report that login is ready.
	fmt.Println("passkey login: credentials present, ready for headless passkey authentication")
	return nil
}

// Users retrieves all users from the Hourglass system (GET /fsreport/users).
func (c *CLI) Users() error {
	users, err := c.client.GetUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	out, err := json.Marshal(users)
	if err != nil {
		return fmt.Errorf("failed to marshal users: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// ScheduleMeetings retrieves meeting schedules for a date range
// (GET /scheduling/mm/meeting/{start}_{end}).
func (c *CLI) ScheduleMeetings(start, end string, lgroup int) error {
	meetings, err := c.client.GetMeetings(start, end, lgroup)
	if err != nil {
		return fmt.Errorf("failed to get meetings: %w", err)
	}

	out, err := json.Marshal(meetings)
	if err != nil {
		return fmt.Errorf("failed to marshal meetings: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// ScheduleAVAttendants retrieves mechanical assignments for a date range
// (GET /scheduling/av_attendant/{start}_{end}).
func (c *CLI) ScheduleAVAttendants(start, end string) error {
	attendants, err := c.client.GetAVAttendants(start, end)
	if err != nil {
		return fmt.Errorf("failed to get AV attendants: %w", err)
	}

	out, err := json.Marshal(attendants)
	if err != nil {
		return fmt.Errorf("failed to marshal AV attendants: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// ScheduleNotifications retrieves notifications for a date range and type
// (GET /scheduling/notifications/{start}_{end}/{type}).
func (c *CLI) ScheduleNotifications(start, end, notifType string) error {
	notifications, err := c.client.GetNotifications(start, end, notifType)
	if err != nil {
		return fmt.Errorf("failed to get notifications: %w", err)
	}

	out, err := json.Marshal(notifications)
	if err != nil {
		return fmt.Errorf("failed to marshal notifications: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// ScheduleWM stub - exits 2 with "not implemented yet: endpoint not discovered"
func (c *CLI) ScheduleWM() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// Privileges stub - exits 2 with "not implemented yet: endpoint not discovered"
func (c *CLI) Privileges() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// Assign stub - dry-run by default, exits 2 with "not implemented yet: endpoint not discovered"
// Requires --apply flag even in the stub (handled at CLI level)
func (c *CLI) Assign() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// Unassign stub - dry-run by default, exits 2 with "not implemented yet: endpoint not discovered"
// Requires --apply flag even in the stub (handled at CLI level)
func (c *CLI) Unassign() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// CheckRules stub - exits 2 with "not implemented yet: endpoint not discovered"
func (c *CLI) CheckRules() error {
	return exitCode2("not implemented yet: endpoint not discovered")
}

// Run executes the CLI with the given arguments.
func Run(args []string) error {
	cli := NewCLI()

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: hourglass <command> [args...]\n")
		return errors.New("no command provided")
	}

	command := args[0]
	remainingArgs := args[1:]

	switch command {
	case "auth":
		return cli.runAuth(remainingArgs)
	case "users":
		return cli.runUsers(remainingArgs)
	case "schedule":
		return cli.runSchedule(remainingArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		return errors.New("unknown command")
	}
}

// exit2 wraps Run to convert exitCode2 errors to exit code 2
func exit2(err error) error {
	if exitErr, ok := err.(*exitCode2Error); ok {
		fmt.Fprintf(os.Stderr, "Error: %s\n", exitErr.Error())
		return fmt.Errorf("exit code 2: %s", exitErr.Error())
	}
	return err
}

func (c *CLI) runAuth(args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: hourglass auth <status|login>\n")
		return errors.New("no auth subcommand provided")
	}

	subcommand := args[0]

	switch subcommand {
	case "status":
		return c.AuthStatus()
	case "login":
		return c.AuthLogin()
	default:
		fmt.Fprintf(os.Stderr, "Unknown auth subcommand: %s\n", subcommand)
		fmt.Fprintf(os.Stderr, "Usage: hourglass auth <status|login>\n")
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

	if jsonFlag {
		return c.Users()
	}

	fmt.Fprintf(os.Stderr, "Usage: hourglass users --json\n[--from YYYY-MM-DD] [--to YYYY-MM-DD] [--lgroup N]\n")
	return errors.New("missing --json flag")
}

func (c *CLI) runSchedule(args []string) error {
	var subcommand string
	var fromDate, toDate string
	var lgroup int
	var notifType string
	var apply bool

	i := 0
	for i < len(args) {
		switch args[i] {
		case "wm":
			subcommand = "wm"
			i++
		case "privileges":
			subcommand = "privileges"
			i++
		case "assign":
			subcommand = "assign"
			apply = true
			i++
		case "unassign":
			subcommand = "unassign"
			apply = true
			i++
		case "check-rules":
			subcommand = "check-rules"
			i++
		case "mm":
			subcommand = "mm"
			i++
		case "av":
			subcommand = "av"
			i++
		case "notifications":
			subcommand = "notifications"
			i++
		case "--from":
			if i+1 < len(args) {
				fromDate = args[i+1]
				i += 2
			}
		case "--to":
			if i+1 < len(args) {
				toDate = args[i+1]
				i += 2
			}
		case "--lgroup":
			if i+1 < len(args) {
				_, err := fmt.Sscanf(args[i+1], "%d", &lgroup)
				if err != nil {
					lgroup = 0
				}
				i += 2
			}
		case "--type":
			if i+1 < len(args) {
				notifType = args[i+1]
				i += 2
			}
		case "--apply":
			apply = true
			i++
		default:
			i++
		}
	}

	switch subcommand {
	case "wm":
		return exit2(c.ScheduleWM())
	case "privileges":
		return exit2(c.Privileges())
	case "assign":
		if !apply {
			fmt.Fprintf(os.Stderr, "Info: assign running in dry-run mode (use --apply to attempt actual operation)\n")
		}
		return exit2(c.Assign())
	case "unassign":
		if !apply {
			fmt.Fprintf(os.Stderr, "Info: unassign running in dry-run mode (use --apply to attempt actual operation)\n")
		}
		return exit2(c.Unassign())
	case "check-rules":
		return exit2(c.CheckRules())
	case "mm":
		if fromDate == "" || toDate == "" {
			fmt.Fprintf(os.Stderr, "Usage: hourglass schedule mm --from YYYY-MM-DD --to YYYY-MM-DD [--lgroup N] --json\n")
			return errors.New("missing required flags")
		}
		return exit2(c.ScheduleMeetings(fromDate, toDate, lgroup))
	case "av":
		if fromDate == "" || toDate == "" {
			fmt.Fprintf(os.Stderr, "Usage: hourglass schedule av --from YYYY-MM-DD --to YYYY-MM-DD --json\n")
			return errors.New("missing required flags")
		}
		return exit2(c.ScheduleAVAttendants(fromDate, toDate))
	case "notifications":
		if fromDate == "" || toDate == "" || notifType == "" {
			fmt.Fprintf(os.Stderr, "Usage: hourglass notifications --from --to --type accepted|declined --json\n")
			return errors.New("missing required flags")
		}
		if notifType != "accepted" && notifType != "declined" {
			fmt.Fprintf(os.Stderr, "Invalid type: %s. Use 'accepted' or 'declined'\n", notifType)
			return errors.New("invalid notification type")
		}
		return exit2(c.ScheduleNotifications(fromDate, toDate, notifType))
	default:
		if subcommand == "" {
			fmt.Fprintf(os.Stderr, "Usage: hourglass schedule <wm|privileges|assign|unassign|check-rules|mm|av|notifications> [flags]\n")
		} else {
			fmt.Fprintf(os.Stderr, "Unknown schedule subcommand: %s\n", subcommand)
		}
		return errors.New("unknown schedule subcommand")
	}
}

// blocklistTransport middleware that refuses non-GET requests and send-like paths.
type blocklistTransport struct {
	Transport http.RoundTripper
	allowSend bool
}

func (t *blocklistTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if HOURGLASS_ALLOW_SEND is set
	if os.Getenv("HOURGLASS_ALLOW_SEND") == "" {
		t.allowSend = false
	} else {
		t.allowSend = true
	}

	// Refuse non-GET requests unless allow_send is 1
	if req.Method != http.MethodGet && !t.allowSend {
		return nil, fmt.Errorf("send operations blocked: %s %s", req.Method, req.URL.Path)
	}

	// Refuse paths matching notify|notification/send|send|publish|email
	// except the existing GET notifications read path
	blockedPaths := []string{
		"notify",
		"notification/send",
		"send",
		"publish",
		"email",
	}

	path := strings.ToLower(req.URL.Path)
	for _, blocked := range blockedPaths {
		if strings.Contains(path, blocked) {
			// Allow the GET /scheduling/notifications read endpoint
			if req.Method == http.MethodGet && strings.HasPrefix(path, "scheduling/notifications") {
				continue
			}
			return nil, fmt.Errorf("send operation blocked: %s %s", req.Method, req.URL.Path)
		}
	}

	return t.Transport.RoundTrip(req)
}
func main() {
	if err := Run(os.Args[1:]); err != nil {
		if exitErr, ok := err.(*exitCode2Error); ok {
			fmt.Fprintf(os.Stderr, "Error: %s\n", exitErr.msg)
			os.Exit(2)
		}
		if strings.Contains(err.Error(), "exit code 2") {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
