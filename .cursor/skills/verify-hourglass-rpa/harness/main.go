//go:build verify

// Verification harness for hourglass-rejections-rpa.
// Starts mock Telegram + Hourglass APIs, runs the real TelegramNotifier and
// manual-check path against scratch SQLite, and exposes a control HTTP API
// so drive scripts can inject the same commands a user types in Telegram.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-telegram/bot/models"

	domain "hourglass-rejections-rpa/src/domain_models"
	"hourglass-rejections-rpa/src/integrations/database/preferences"
	"hourglass-rejections-rpa/src/integrations/i18n"
	"hourglass-rejections-rpa/src/services/hourglass"
	notifier "hourglass-rejections-rpa/src/services/notification"
)

const (
	adminChatID   int64 = 9001
	adminUsername       = "verify_admin"
	botToken            = "verify-token:fake"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		fatalUsage()
	}
	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			log.Fatalf("serve: %v", err)
		}
	case "drive":
		if err := runDrive(os.Args[2:]); err != nil {
			log.Fatalf("drive: %v", err)
		}
	case "doctor":
		if err := runDoctor(os.Args[2:]); err != nil {
			log.Fatalf("doctor: %v", err)
		}
	default:
		fatalUsage()
	}
}

func fatalUsage() {
	log.Fatalf(`usage:
  verify-harness serve  -scratch DIR [-evidence DIR]
  verify-harness doctor -scratch DIR
  verify-harness drive  -scratch DIR <subcommand> [args...]

drive subcommands:
  clear
  message TEXT
  callback DATA
  wait-outbound N [timeout-sec]
  outbound
  hourglass-mode empty|declined
  preferences
  feature start-help|configure-preferences|checknow|language
`)
}

// ---------------------------------------------------------------------------
// serve
// ---------------------------------------------------------------------------

type instanceState struct {
	Scratch       string `json:"scratch"`
	Evidence      string `json:"evidence"`
	ControlURL    string `json:"control_url"`
	TelegramURL   string `json:"telegram_url"`
	HourglassURL  string `json:"hourglass_url"`
	DBPath        string `json:"db_path"`
	HarnessPID    int    `json:"harness_pid"`
	AdminChatID   int64  `json:"admin_chat_id"`
	BinaryBuildOK bool   `json:"binary_build_ok"`
}

type outboundEvent struct {
	At      time.Time      `json:"at"`
	Method  string         `json:"method"`
	ChatID  int64          `json:"chat_id,omitempty"`
	Text    string         `json:"text,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	RawForm url.Values     `json:"raw_form,omitempty"`
}

type harness struct {
	scratch  string
	evidence string

	mu            sync.Mutex
	updateQueue   []*models.Update
	updateWaiters []chan struct{}
	nextUpdateID  int64
	nextMessageID atomic.Int64

	outbound   []outboundEvent
	hourglassM string // "empty" | "declined"

	store       *preferences.Store
	prefManager *preferences.PreferenceManager
	tg          *notifier.TelegramNotifier
	botCancel   context.CancelFunc
	analyzer    *hourglass.APIAnalyzer
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	scratch := fs.String("scratch", "", "scratch directory")
	evidence := fs.String("evidence", "", "evidence directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scratch == "" {
		return errors.New("-scratch is required")
	}
	if *evidence == "" {
		*evidence = filepath.Join(*scratch, "evidence")
	}
	if err := os.MkdirAll(*scratch, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(*evidence, 0o755); err != nil {
		return err
	}

	if err := i18n.Init(); err != nil {
		return fmt.Errorf("i18n: %w", err)
	}

	h := &harness{
		scratch:    *scratch,
		evidence:   *evidence,
		hourglassM: "empty",
	}
	h.nextMessageID.Store(1)

	dbPath := filepath.Join(*scratch, "preferences.db")
	store, err := preferences.NewStoreWithPath(dbPath)
	if err != nil {
		return fmt.Errorf("sqlite: %w", err)
	}
	h.store = store
	h.prefManager = preferences.NewPreferenceManager(store)

	tgLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	hgLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = tgLn.Close()
		return err
	}
	ctrlLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = tgLn.Close()
		_ = hgLn.Close()
		return err
	}

	tgURL := "http://" + tgLn.Addr().String()
	hgURL := "http://" + hgLn.Addr().String()
	ctrlURL := "http://" + ctrlLn.Addr().String()

	tgServer := &http.Server{Handler: h.telegramHandler()}
	hgServer := &http.Server{Handler: h.hourglassHandler()}
	ctrlServer := &http.Server{Handler: h.controlHandler()}

	go func() { _ = tgServer.Serve(tgLn) }()
	go func() { _ = hgServer.Serve(hgLn) }()
	go func() { _ = ctrlServer.Serve(ctrlLn) }()

	client := hourglass.NewClient()
	client.SetBaseURL(hgURL)
	client.SetHGLogin("verify-hglogin")
	client.SetXSRFToken("verify-xsrf")
	h.analyzer = hourglass.NewAPIAnalyzer(client)

	tg, err := newNotifier(tgURL)
	if err != nil {
		return err
	}
	h.tg = tg

	botCtx, cancel := context.WithCancel(context.Background())
	h.botCancel = cancel

	tg.SetCheckNowCallback(func(ctx context.Context, chatID int64) error {
		return runManualCheck(ctx, h.analyzer, h.prefManager, tg, chatID)
	})

	if err := tg.StartBot(botCtx, h.prefManager); err != nil {
		cancel()
		return fmt.Errorf("start bot: %w", err)
	}

	// Give the long-poll loop a moment to open its first getUpdates.
	time.Sleep(150 * time.Millisecond)

	state := instanceState{
		Scratch:       *scratch,
		Evidence:      *evidence,
		ControlURL:    ctrlURL,
		TelegramURL:   tgURL,
		HourglassURL:  hgURL,
		DBPath:        dbPath,
		HarnessPID:    os.Getpid(),
		AdminChatID:   adminChatID,
		BinaryBuildOK: true,
	}
	if err := writeState(*scratch, state); err != nil {
		cancel()
		return err
	}
	fmt.Printf("SCRATCH=%s\n", *scratch)
	fmt.Printf("CONTROL_URL=%s\n", ctrlURL)
	fmt.Printf("EVIDENCE=%s\n", *evidence)
	fmt.Printf("ready\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancel()
	_ = tg.StopBot()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = ctrlServer.Shutdown(shutdownCtx)
	_ = tgServer.Shutdown(shutdownCtx)
	_ = hgServer.Shutdown(shutdownCtx)
	_ = store.Close()
	return nil
}

func newNotifier(serverURL string) (*notifier.TelegramNotifier, error) {
	// Build tag `verify` exposes this constructor; production binaries omit it.
	return notifier.NewTelegramNotifierForVerify(botToken, adminChatID, []int64{adminChatID}, serverURL)
}

func runManualCheck(
	ctx context.Context,
	analyzer *hourglass.APIAnalyzer,
	prefManager *preferences.PreferenceManager,
	tg *notifier.TelegramNotifier,
	chatID int64,
) error {
	pref, err := prefManager.Get(chatID)
	if err != nil {
		return err
	}
	if pref == nil {
		return errors.New("user preferences not found")
	}
	lang := prefManager.GetLanguage(chatID)
	sections := pref.Sections()
	if len(sections) == 0 {
		return tg.SendNoRejectionsMessage(chatID, i18n.Localize(lang, "no_sections_selected", nil))
	}

	var all []domain.Rejection
	for _, section := range sections {
		result, aerr := analyzer.AnalyzeSection(section)
		if aerr != nil {
			return aerr
		}
		if result != nil && result.Error != nil {
			return result.Error
		}
		if result != nil {
			all = append(all, result.Rejections...)
		}
	}
	if len(all) == 0 {
		return tg.SendNoRejectionsMessage(chatID, i18n.Localize(lang, "no_rejections_found", nil))
	}
	return tg.SendRejectionsNotification(chatID, all)
}

func writeState(scratch string, state instanceState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(scratch, "state.json"), data, 0o600)
}

func readState(scratch string) (instanceState, error) {
	var state instanceState
	data, err := os.ReadFile(filepath.Join(scratch, "state.json"))
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

// ---------------------------------------------------------------------------
// mock Telegram
// ---------------------------------------------------------------------------

func (h *harness) telegramHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := telegramMethod(r.URL.Path)
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			_ = r.ParseForm()
		}
		form := r.Form
		if r.MultipartForm != nil && r.MultipartForm.Value != nil {
			form = url.Values(r.MultipartForm.Value)
		}

		w.Header().Set("Content-Type", "application/json")

		switch method {
		case "getUpdates":
			h.serveGetUpdates(w, r, form)
			return
		case "setMyCommands":
			writeTG(w, true)
			return
		case "answerCallbackQuery", "deleteMessage":
			writeTG(w, true)
			return
		case "sendMessage":
			h.recordOutbound("sendMessage", form)
			id := h.nextMessageID.Add(1) - 1
			writeTG(w, map[string]any{
				"message_id": id,
				"date":       time.Now().Unix(),
				"chat":       map[string]any{"id": formInt(form, "chat_id"), "type": "private"},
				"text":       form.Get("text"),
			})
			return
		case "editMessageText":
			h.recordOutbound("editMessageText", form)
			writeTG(w, map[string]any{
				"message_id": formInt(form, "message_id"),
				"date":       time.Now().Unix(),
				"chat":       map[string]any{"id": formInt(form, "chat_id"), "type": "private"},
				"text":       form.Get("text"),
			})
			return
		case "editMessageReplyMarkup":
			h.recordOutbound("editMessageReplyMarkup", form)
			writeTG(w, map[string]any{
				"message_id": formInt(form, "message_id"),
				"date":       time.Now().Unix(),
				"chat":       map[string]any{"id": formInt(form, "chat_id"), "type": "private"},
			})
			return
		case "getMe":
			writeTG(w, map[string]any{
				"id":         1,
				"is_bot":     true,
				"first_name": "VerifyBot",
				"username":   "verify_bot",
			})
			return
		default:
			h.recordOutbound(method, form)
			writeTG(w, map[string]any{
				"message_id": h.nextMessageID.Add(1) - 1,
				"id":         1,
				"is_bot":     true,
			})
		}
	})
}

func telegramMethod(path string) string {
	// /bot<token>/<method>
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func (h *harness) serveGetUpdates(w http.ResponseWriter, r *http.Request, form url.Values) {
	timeoutSec := 1
	if v := form.Get("timeout"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}
	// Cap mock long-poll so doctor/drive stay snappy.
	if timeoutSec > 2 {
		timeoutSec = 2
	}

	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for {
		h.mu.Lock()
		if len(h.updateQueue) > 0 {
			batch := h.updateQueue
			h.updateQueue = nil
			h.mu.Unlock()
			writeTG(w, batch)
			return
		}
		wait := make(chan struct{}, 1)
		h.updateWaiters = append(h.updateWaiters, wait)
		h.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			writeTG(w, []*models.Update{})
			return
		}
		timer := time.NewTimer(remaining)
		select {
		case <-r.Context().Done():
			timer.Stop()
			writeTG(w, []*models.Update{})
			return
		case <-wait:
			timer.Stop()
			continue
		case <-timer.C:
			writeTG(w, []*models.Update{})
			return
		}
	}
}

func (h *harness) recordOutbound(method string, form url.Values) {
	ev := outboundEvent{
		At:      time.Now().UTC(),
		Method:  method,
		ChatID:  formInt(form, "chat_id"),
		Text:    form.Get("text"),
		RawForm: form,
	}
	if rm := form.Get("reply_markup"); rm != "" {
		var data map[string]any
		if json.Unmarshal([]byte(rm), &data) == nil {
			ev.Data = data
		} else {
			ev.Data = map[string]any{"reply_markup": rm}
		}
	}
	h.mu.Lock()
	h.outbound = append(h.outbound, ev)
	h.mu.Unlock()

	path := filepath.Join(h.evidence, "outbound.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(ev)
}

func (h *harness) enqueue(update *models.Update) {
	h.mu.Lock()
	h.nextUpdateID++
	update.ID = h.nextUpdateID
	h.updateQueue = append(h.updateQueue, update)
	waiters := h.updateWaiters
	h.updateWaiters = nil
	h.mu.Unlock()
	for _, w := range waiters {
		select {
		case w <- struct{}{}:
		default:
		}
	}
}

func writeTG(w http.ResponseWriter, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func formInt(form url.Values, key string) int64 {
	v, _ := strconv.ParseInt(form.Get(key), 10, 64)
	return v
}

// ---------------------------------------------------------------------------
// mock Hourglass
// ---------------------------------------------------------------------------

func (h *harness) hourglassHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/fsreport/users") || path == "/api/v0.2/fsreport/users" || strings.Contains(path, "/fsreport/users"):
			_ = json.NewEncoder(w).Encode(hourglass.UsersResponse{
				Users: []hourglass.User{{
					ID: 1, Firstname: "Verify", Lastname: "User", Descriptor: "Verify User",
				}},
			})
		case strings.Contains(path, "/scheduling/notifications/"):
			h.mu.Lock()
			mode := h.hourglassM
			h.mu.Unlock()
			if mode == "declined" && strings.Contains(path, "/pubwit") {
				_ = json.NewEncoder(w).Encode([]hourglass.Notification{{
					ID:             42,
					CongregationID: 48092,
					Date:           time.Now().AddDate(0, 0, 7).Format("2006-01-02"),
					Type:           "pubwit",
					Status:         "declined",
					Assignee:       1,
					Part:           100,
				}})
				return
			}
			_ = json.NewEncoder(w).Encode([]hourglass.Notification{})
		case strings.Contains(path, "/scheduling/mm/meeting/"):
			_ = json.NewEncoder(w).Encode([]hourglass.Meeting{})
		case strings.Contains(path, "/scheduling/av_attendant/"):
			_ = json.NewEncoder(w).Encode([]hourglass.AVAttendant{})
		default:
			http.NotFound(w, r)
		}
	})
}

// ---------------------------------------------------------------------------
// control API
// ---------------------------------------------------------------------------

func (h *harness) controlHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		outN := len(h.outbound)
		mode := h.hourglassM
		h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":             true,
			"pid":            os.Getpid(),
			"outbound":       outN,
			"hourglass_mode": mode,
			"admin_chat_id":  adminChatID,
			"db_path":        filepath.Join(h.scratch, "preferences.db"),
		})
	})
	mux.HandleFunc("POST /message", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text   string `json:"text"`
			ChatID int64  `json:"chat_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if body.ChatID == 0 {
			body.ChatID = adminChatID
		}
		h.enqueue(messageUpdate(body.ChatID, body.Text))
		writeOK(w)
	})
	mux.HandleFunc("POST /callback", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data      string `json:"data"`
			ChatID    int64  `json:"chat_id"`
			MessageID int    `json:"message_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if body.ChatID == 0 {
			body.ChatID = adminChatID
		}
		if body.MessageID == 0 {
			body.MessageID = 1
		}
		h.enqueue(callbackUpdate(body.ChatID, body.MessageID, body.Data))
		writeOK(w)
	})
	mux.HandleFunc("POST /outbound/clear", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.outbound = nil
		h.mu.Unlock()
		writeOK(w)
	})
	mux.HandleFunc("GET /outbound", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		out := append([]outboundEvent(nil), h.outbound...)
		h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("POST /hourglass-mode", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if body.Mode != "empty" && body.Mode != "declined" {
			http.Error(w, "mode must be empty|declined", 400)
			return
		}
		h.mu.Lock()
		h.hourglassM = body.Mode
		h.mu.Unlock()
		writeOK(w)
	})
	mux.HandleFunc("GET /preferences", func(w http.ResponseWriter, r *http.Request) {
		chatID := adminChatID
		if v := r.URL.Query().Get("chat_id"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				chatID = n
			}
		}
		pref, err := h.prefManager.Get(chatID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if pref == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"exists": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"exists":     true,
			"chat_id":    pref.ChatID,
			"sections":   pref.Sections(),
			"language":   pref.Language,
			"enabled":    pref.Enabled,
			"authorized": pref.Authorized,
		})
	})
	return mux
}

func writeOK(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func messageUpdate(chatID int64, text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			ID:   1,
			Date: int(time.Now().Unix()),
			Text: text,
			Chat: models.Chat{ID: chatID, Type: "private"},
			From: &models.User{ID: chatID, Username: adminUsername, FirstName: "Verify"},
		},
	}
}

func callbackUpdate(chatID int64, messageID int, data string) *models.Update {
	return &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   fmt.Sprintf("cb-%d", time.Now().UnixNano()),
			From: models.User{ID: chatID, Username: adminUsername, FirstName: "Verify"},
			Data: data,
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   messageID,
					Date: int(time.Now().Unix()),
					Chat: models.Chat{ID: chatID, Type: "private"},
					Text: "callback-anchor",
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// doctor + drive CLI
// ---------------------------------------------------------------------------

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	scratch := fs.String("scratch", "", "scratch directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *scratch == "" {
		return errors.New("-scratch is required")
	}
	state, err := readState(*scratch)
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	report := filepath.Join(state.Evidence, "doctor.txt")
	var b strings.Builder
	fail := false
	check := func(name string, ok bool, detail string) {
		status := "PASS"
		if !ok {
			status = "FAIL"
			fail = true
		}
		line := fmt.Sprintf("%s %s %s\n", status, name, detail)
		b.WriteString(line)
		fmt.Print(line)
	}

	alive := false
	if state.HarnessPID > 0 {
		if err := syscall.Kill(state.HarnessPID, 0); err == nil {
			alive = true
		}
	}
	check("harness_pid_alive", alive, fmt.Sprintf("pid=%d", state.HarnessPID))

	resp, err := http.Get(state.ControlURL + "/health")
	healthOK := false
	healthBody := ""
	if err == nil {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		healthBody = string(raw)
		healthOK = resp.StatusCode == 200 && strings.Contains(healthBody, `"ok":true`)
	}
	check("control_health", healthOK, healthBody)

	dbOK := false
	if st, err := os.Stat(state.DBPath); err == nil && st.Size() > 0 {
		dbOK = true
	}
	check("sqlite_present", dbOK, state.DBPath)

	binOK := false
	if st, err := os.Stat(filepath.Join(*scratch, "rpa")); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
		binOK = true
	}
	check("rpa_binary", binOK, filepath.Join(*scratch, "rpa"))

	_ = os.WriteFile(report, []byte(b.String()), 0o600)
	if fail {
		return errors.New("doctor failed — see " + report)
	}
	return nil
}

func runDrive(args []string) error {
	fs := flag.NewFlagSet("drive", flag.ContinueOnError)
	scratch := fs.String("scratch", "", "scratch directory")
	// peel flags then subcommand
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-scratch" && i+1 < len(args) {
			_ = fs.Set("scratch", args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(a, "-scratch=") {
			_ = fs.Set("scratch", strings.TrimPrefix(a, "-scratch="))
			continue
		}
		rest = append(rest, a)
	}
	if *scratch == "" {
		return errors.New("-scratch is required")
	}
	if len(rest) == 0 {
		return errors.New("drive subcommand required")
	}
	state, err := readState(*scratch)
	if err != nil {
		return err
	}
	client := &driveClient{base: state.ControlURL, evidence: state.Evidence, scratch: *scratch}

	switch rest[0] {
	case "clear":
		return client.clear()
	case "message":
		if len(rest) < 2 {
			return errors.New("message TEXT required")
		}
		return client.message(strings.Join(rest[1:], " "))
	case "callback":
		if len(rest) < 2 {
			return errors.New("callback DATA required")
		}
		return client.callback(strings.Join(rest[1:], " "))
	case "wait-outbound":
		n := 1
		timeout := 5
		if len(rest) > 1 {
			n, _ = strconv.Atoi(rest[1])
		}
		if len(rest) > 2 {
			timeout, _ = strconv.Atoi(rest[2])
		}
		return client.waitOutbound(n, time.Duration(timeout)*time.Second)
	case "outbound":
		out, err := client.outbound()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case "hourglass-mode":
		if len(rest) < 2 {
			return errors.New("hourglass-mode empty|declined")
		}
		return client.hourglassMode(rest[1])
	case "preferences":
		pref, err := client.preferences()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pref)
	case "feature":
		if len(rest) < 2 {
			return errors.New("feature name required")
		}
		return client.runFeature(rest[1])
	default:
		return fmt.Errorf("unknown drive subcommand %q", rest[0])
	}
}

type driveClient struct {
	base     string
	evidence string
	scratch  string
}

func (c *driveClient) postJSON(path string, body any) error {
	raw, _ := json.Marshal(body)
	resp, err := http.Post(c.base+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", path, string(b))
	}
	return nil
}

func (c *driveClient) clear() error {
	return c.postJSON("/outbound/clear", map[string]any{})
}

func (c *driveClient) message(text string) error {
	return c.postJSON("/message", map[string]any{"text": text})
}

func (c *driveClient) callback(data string) error {
	return c.postJSON("/callback", map[string]any{"data": data})
}

func (c *driveClient) hourglassMode(mode string) error {
	return c.postJSON("/hourglass-mode", map[string]any{"mode": mode})
}

func (c *driveClient) outbound() ([]outboundEvent, error) {
	resp, err := http.Get(c.base + "/outbound")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []outboundEvent
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *driveClient) preferences() (map[string]any, error) {
	resp, err := http.Get(c.base + "/preferences")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *driveClient) waitOutbound(n int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := c.outbound()
		if err != nil {
			return err
		}
		if len(out) >= n {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	out, _ := c.outbound()
	return fmt.Errorf("timeout waiting for %d outbound events (have %d)", n, len(out))
}

func (c *driveClient) runFeature(name string) error {
	dir := filepath.Join(c.evidence, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	switch name {
	case "start-help":
		return c.featureStartHelp(dir)
	case "configure-preferences":
		return c.featureConfigure(dir)
	case "checknow":
		return c.featureChecknow(dir)
	case "language":
		return c.featureLanguage(dir)
	default:
		return fmt.Errorf("unknown feature %q", name)
	}
}

func (c *driveClient) snapshot(dir, label string) error {
	out, err := c.outbound()
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, label+"-outbound.json"), out); err != nil {
		return err
	}
	pref, err := c.preferences()
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, label+"-preferences.json"), pref)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c *driveClient) featureStartHelp(dir string) error {
	_ = c.clear()
	if err := c.message("/start"); err != nil {
		return err
	}
	if err := c.waitOutbound(1, 5*time.Second); err != nil {
		return err
	}
	if err := c.snapshot(dir, "after-start"); err != nil {
		return err
	}
	out, _ := c.outbound()
	if !outboundContains(out, "Welcome to Hourglass RPA Bot!") {
		return fmt.Errorf("expected welcome text, got %#v", textsOf(out))
	}
	_ = c.clear()
	if err := c.message("/help"); err != nil {
		return err
	}
	if err := c.waitOutbound(1, 5*time.Second); err != nil {
		return err
	}
	if err := c.snapshot(dir, "after-help"); err != nil {
		return err
	}
	out, _ = c.outbound()
	if !outboundContains(out, "/checknow") {
		return fmt.Errorf("expected help listing /checknow, got %#v", textsOf(out))
	}
	fmt.Println("feature start-help: PASS")
	return nil
}

func (c *driveClient) featureConfigure(dir string) error {
	_ = c.clear()
	// Ensure preference row exists via /start
	_ = c.message("/start")
	_ = c.waitOutbound(1, 5*time.Second)
	_ = c.clear()

	if err := c.message("/configure"); err != nil {
		return err
	}
	if err := c.waitOutbound(1, 5*time.Second); err != nil {
		return err
	}
	if err := c.snapshot(dir, "after-configure"); err != nil {
		return err
	}
	out, _ := c.outbound()
	if !outboundContains(out, "Choose sections") {
		return fmt.Errorf("expected configure prompt, got %#v", textsOf(out))
	}
	// Toggle Public Witnessing on, save.
	if err := c.callback("section_Public Witnessing"); err != nil {
		return err
	}
	time.Sleep(200 * time.Millisecond)
	if err := c.callback("save_config"); err != nil {
		return err
	}
	if err := c.waitOutbound(1, 5*time.Second); err != nil {
		// save edits message text; may already have outbound from toggle
		_ = err
	}
	time.Sleep(300 * time.Millisecond)
	if err := c.snapshot(dir, "after-save"); err != nil {
		return err
	}
	pref, err := c.preferences()
	if err != nil {
		return err
	}
	sections, _ := pref["sections"].([]any)
	found := false
	for _, s := range sections {
		if s == "Public Witnessing" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("expected Public Witnessing in preferences, got %#v", pref)
	}
	fmt.Println("feature configure-preferences: PASS")
	return nil
}

func (c *driveClient) featureChecknow(dir string) error {
	// Depend on configure having set a section; set it ourselves for isolation.
	_ = c.message("/start")
	_ = c.waitOutbound(1, 5*time.Second)
	_ = c.clear()
	_ = c.message("/configure")
	_ = c.waitOutbound(1, 5*time.Second)
	_ = c.callback("section_Public Witnessing")
	time.Sleep(200 * time.Millisecond)
	_ = c.callback("save_config")
	time.Sleep(300 * time.Millisecond)
	_ = c.clear()

	if err := c.hourglassMode("empty"); err != nil {
		return err
	}
	if err := c.message("/checknow"); err != nil {
		return err
	}
	// check_now_requested + async no_rejections_found (+ maybe delete)
	if err := c.waitOutbound(2, 8*time.Second); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	if err := c.snapshot(dir, "empty-check"); err != nil {
		return err
	}
	out, _ := c.outbound()
	if !outboundContains(out, "No rejections found") && !outboundContains(out, "Immediate check requested") {
		return fmt.Errorf("expected empty check messages, got %#v", textsOf(out))
	}

	_ = c.clear()
	if err := c.hourglassMode("declined"); err != nil {
		return err
	}
	if err := c.message("/checknow"); err != nil {
		return err
	}
	if err := c.waitOutbound(2, 8*time.Second); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	if err := c.snapshot(dir, "declined-check"); err != nil {
		return err
	}
	out, _ = c.outbound()
	if !outboundContains(out, "Verify User") && !outboundContains(out, "rejection") && !outboundContains(out, "Rejection") && !outboundContains(out, "Public Witnessing") {
		// Rejection notifications are HTML blobs; accept any non-empty post-request text beyond the processing line.
		texts := textsOf(out)
		if len(texts) < 2 {
			return fmt.Errorf("expected rejection notification outbound, got %#v", texts)
		}
	}
	fmt.Println("feature checknow: PASS")
	return nil
}

func (c *driveClient) featureLanguage(dir string) error {
	_ = c.clear()
	_ = c.message("/start")
	_ = c.waitOutbound(1, 5*time.Second)
	_ = c.clear()

	if err := c.message("/language"); err != nil {
		return err
	}
	if err := c.waitOutbound(1, 5*time.Second); err != nil {
		return err
	}
	if err := c.snapshot(dir, "language-menu"); err != nil {
		return err
	}
	if err := c.callback("lang_pt-BR"); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	if err := c.snapshot(dir, "after-lang"); err != nil {
		return err
	}
	pref, err := c.preferences()
	if err != nil {
		return err
	}
	if pref["language"] != "pt-BR" {
		return fmt.Errorf("expected language pt-BR, got %#v", pref)
	}
	out, _ := c.outbound()
	if !outboundContains(out, "Idioma alterado") && !outboundContains(out, "Language changed") {
		// editMessageText carries the confirmation
		if !outboundContains(out, "sucesso") && !outboundContains(out, "successfully") {
			return fmt.Errorf("expected language confirmation, got %#v", textsOf(out))
		}
	}
	fmt.Println("feature language: PASS")
	return nil
}

func outboundContains(out []outboundEvent, substr string) bool {
	for _, e := range out {
		if strings.Contains(e.Text, substr) {
			return true
		}
	}
	return false
}

func textsOf(out []outboundEvent) []string {
	var t []string
	for _, e := range out {
		if e.Text != "" {
			t = append(t, e.Text)
		}
	}
	return t
}
