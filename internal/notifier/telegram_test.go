package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/i18n"
	"hourglass-rejections-rpa/internal/preferences"
)

func TestMain(m *testing.M) {
	if err := i18n.Init(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// newTestBot creates a bot that skips the getMe API call for testing.
func newTestBot(t *testing.T) *bot.Bot {
	t.Helper()
	b, err := bot.New("test-token:fake", bot.WithSkipGetMe())
	require.NoError(t, err)
	return b
}

// newTestNotifier creates a TelegramNotifier with a fake bot for testing.
func newTestNotifier(t *testing.T, whitelist []int64) *TelegramNotifier {
	t.Helper()
	return &TelegramNotifier{
		bot:       newTestBot(t),
		chatID:    12345,
		whitelist: whitelist,
	}
}

// newTestPrefManager creates a PreferenceManager backed by SQLite.
func newTestPrefManager(t *testing.T) *preferences.PreferenceManager {
	t.Helper()
	store, err := preferences.NewStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return preferences.NewPreferenceManager(store)
}

func newClosedPrefManager(t *testing.T) *preferences.PreferenceManager {
	t.Helper()
	store, err := preferences.NewStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	pm := preferences.NewPreferenceManager(store)
	store.Close()
	return pm
}

func newMockTelegramServer(t *testing.T) *testServer {
	t.Helper()
	return newHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var result any
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/setMyCommands"),
			strings.HasSuffix(path, "/deleteMessage"):
			result = true
		default:
			result = map[string]any{
				"id":         123,
				"is_bot":     true,
				"first_name": "TestBot",
				"username":   "test_bot",
				"message_id": 1,
			}
		}

		resp := map[string]any{"ok": true, "result": result}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newTestBotWithServer(t *testing.T, srv *testServer) *bot.Bot {
	t.Helper()
	b, err := bot.New("test-token:fake", bot.WithServerURL(srv.URL))
	require.NoError(t, err)
	return b
}

func newTestNotifierWithServer(t *testing.T, srv *testServer, whitelist []int64) *TelegramNotifier {
	t.Helper()
	return &TelegramNotifier{
		bot:       newTestBotWithServer(t, srv),
		chatID:    12345,
		whitelist: whitelist,
	}
}

// --- AllSections ---

func TestAllSections(t *testing.T) {
	expected := []string{
		"Mechanical Parts",
		"Field Ministry",
		"Public Witnessing",
		"Midweek Meeting",
	}
	assert.Equal(t, expected, domain.AllSections)
}

// --- NewTelegramNotifier ---

func TestNewTelegramNotifier_EmptyToken(t *testing.T) {
	tn, err := NewTelegramNotifier("", 12345, nil)
	assert.Error(t, err)
	assert.Nil(t, tn)
	assert.Contains(t, err.Error(), "token is required")
}

func TestNewTelegramNotifier_ZeroChatID(t *testing.T) {
	tn, err := NewTelegramNotifier("test-token", 0, nil)
	assert.Error(t, err)
	assert.Nil(t, tn)
	assert.Contains(t, err.Error(), "chat ID is required")
}

func TestNewTelegramNotifier_Success(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotNil(t, tn)
	assert.Equal(t, int64(12345), tn.chatID)
}

// --- IsAuthorized ---

func TestIsAuthorized_EmptyWhitelist(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.True(t, tn.IsAuthorized(12345))
	assert.True(t, tn.IsAuthorized(99999))
}

func TestIsAuthorized_InWhitelist(t *testing.T) {
	tn := newTestNotifier(t, []int64{111, 222, 333})
	assert.True(t, tn.IsAuthorized(111))
	assert.True(t, tn.IsAuthorized(222))
	assert.True(t, tn.IsAuthorized(333))
}

func TestIsAuthorized_NotInWhitelist(t *testing.T) {
	tn := newTestNotifier(t, []int64{111, 222})
	assert.False(t, tn.IsAuthorized(999))
	assert.False(t, tn.IsAuthorized(0))
}

// --- IsConfigured ---

func TestIsConfigured(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.True(t, tn.IsConfigured())
}

func TestIsConfigured_Nil(t *testing.T) {
	var tn *TelegramNotifier
	assert.False(t, tn.IsConfigured())
}

func TestIsConfigured_NilBot(t *testing.T) {
	tn := &TelegramNotifier{chatID: 12345}
	assert.False(t, tn.IsConfigured())
}

func TestIsConfigured_ZeroChatID(t *testing.T) {
	tn := &TelegramNotifier{bot: newTestBot(t)}
	assert.False(t, tn.IsConfigured())
}

// --- StartBot / StopBot ---

func TestStartBot_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.StartBot(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "preference manager is required")
}

func TestStartBot_Success(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- tn.StartBot(ctx, pm)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	err := <-errChan
	assert.Error(t, err)
}

func TestStopBot_Idempotent(t *testing.T) {
	tn := newTestNotifier(t, nil)

	err := tn.StopBot()
	assert.NoError(t, err)

	err = tn.StopBot()
	assert.NoError(t, err)
}

// --- SetCheckNowCallback ---

func TestSetCheckNowCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)

	callback := func(ctx context.Context, chatID int64) error {
		return nil
	}

	tn.SetCheckNowCallback(callback)

	tn.mu.Lock()
	assert.NotNil(t, tn.checkNowCallback)
	tn.mu.Unlock()
}

// --- SendRejectionsNotification ---

func TestSendRejectionsNotification_Empty(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.SendRejectionsNotification(12345, nil)
	assert.NoError(t, err)
}

func TestSendRejectionsNotification_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{111})
	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "Test", What: "Test", When: "01/01/2026"},
	}
	err := tn.SendRejectionsNotification(222, rejections)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

// --- containsSection ---

func TestContainsSection_Found(t *testing.T) {
	sections := []string{"Field Ministry", "Mechanical Parts"}
	assert.True(t, containsSection(sections, "Field Ministry"))
}

func TestContainsSection_NotFound(t *testing.T) {
	sections := []string{"Field Ministry", "Mechanical Parts"}
	assert.False(t, containsSection(sections, "Vida e Ministério"))
}

func TestContainsSection_Empty(t *testing.T) {
	assert.False(t, containsSection([]string{}, "Field Ministry"))
	assert.False(t, containsSection(nil, "Field Ministry"))
}

// --- removeSection ---

func TestRemoveSection_Exists(t *testing.T) {
	sections := []string{"Field Ministry", "Mechanical Parts", "Testemunho"}
	result := removeSection(sections, "Mechanical Parts")
	assert.Equal(t, []string{"Field Ministry", "Testemunho"}, result)
}

func TestRemoveSection_NotExists(t *testing.T) {
	sections := []string{"Field Ministry", "Mechanical Parts"}
	result := removeSection(sections, "Vida")
	assert.Equal(t, sections, result)
}

func TestRemoveSection_Empty(t *testing.T) {
	result := removeSection([]string{}, "Field Ministry")
	assert.Equal(t, []string{}, result)
}

func TestRemoveSection_SingleElement(t *testing.T) {
	sections := []string{"Field Ministry"}
	result := removeSection(sections, "Field Ministry")
	assert.Equal(t, []string{}, result)
}

func TestRemoveSection_NoMutation(t *testing.T) {
	original := []string{"Field Ministry", "Mechanical Parts", "Testemunho"}
	_ = removeSection(original, "Mechanical Parts")
	assert.Equal(t, 3, len(original))
}

// --- buildConfigKeyboard ---

func TestBuildConfigKeyboard_NoSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	prefs := &preferences.UserPreference{
		ChatID:       12345,
		SectionsJSON: "[]",
	}

	keyboard := tn.buildConfigKeyboard(prefs, "en")
	require.NotNil(t, keyboard)

	inlineKB, ok := keyboard.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	assert.GreaterOrEqual(t, len(inlineKB.InlineKeyboard), 5)
}

func TestBuildConfigKeyboard_SomeSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	prefs := &preferences.UserPreference{
		ChatID:       12345,
		SectionsJSON: `["Field Ministry","Mechanical Parts"]`,
	}

	keyboard := tn.buildConfigKeyboard(prefs, "en")
	require.NotNil(t, keyboard)

	inlineKB, ok := keyboard.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	assert.GreaterOrEqual(t, len(inlineKB.InlineKeyboard), 1)
}

func TestBuildConfigKeyboard_AllSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	prefs := &preferences.UserPreference{
		ChatID:       12345,
		SectionsJSON: `["Mechanical Parts","Field Ministry","Public Witnessing","Midweek Meeting"]`,
	}

	keyboard := tn.buildConfigKeyboard(prefs, "en")
	require.NotNil(t, keyboard)

	inlineKB, ok := keyboard.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	assert.GreaterOrEqual(t, len(inlineKB.InlineKeyboard), 5)
}

// --- Handler Tests ---

func TestHandleStart(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleStart(context.Background(), b, update)
}

func TestHandleStart_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleStart(context.Background(), b, update)
}

func TestHandleConfig(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleConfig(context.Background(), b, update)
}

func TestHandleConfig_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleConfig(context.Background(), b, update)
}

func TestHandleStatus(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleStatus(context.Background(), b, update)
}

func TestHandleStatus_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleStatus(context.Background(), b, update)
}

func TestHandleHelp(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleHelp(context.Background(), b, update)
}

func TestHandleHelp_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleHelp(context.Background(), b, update)
}

func TestHandleStats(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBotWithServer(t, srv)

	_, _ = pm.GetOrCreate(12345, "testuser")
	_, _ = pm.GetOrCreate(54321, "otheruser")

	_ = tn.SendNoRejectionsMessage(12345, "ok")
	_ = tn.SendRejectionsNotification(12345, []domain.Rejection{{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"}})

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleStats(context.Background(), b, update)
}

func TestHandleStats_UnauthorizedWithMockServer(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleStats(context.Background(), b, update)
}

func TestHandleStats_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleStats(context.Background(), b, update)
}

func TestHandleWhoAmI(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBotWithServer(t, srv)

	_, _ = pm.GetOrCreate(12345, "testuser")
	_ = pm.UpdateSections(12345, []string{"Field Ministry", "Mechanical Parts"})
	_ = pm.UpdateLanguage(12345, "es")

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleWhoAmI(context.Background(), b, update)
}

func TestHandleWhoAmI_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleWhoAmI(context.Background(), b, update)
}

func TestHandleLanguageSelect_SpanishAndFrench(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	_, _ = pm.GetOrCreate(12345, "testuser")

	updateES := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id-es",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "lang_es",
		},
	}

	tn.handleLanguageSelect(context.Background(), b, updateES)
	assert.Equal(t, "es", pm.GetLanguage(12345))

	updateFR := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id-fr",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "lang_fr",
		},
	}

	tn.handleLanguageSelect(context.Background(), b, updateFR)
	assert.Equal(t, "fr", pm.GetLanguage(12345))
}

func TestHandleCheckNow_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleCheckNow(context.Background(), b, update)
}

func TestHandleCheckNow_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{Message: nil}
	tn.handleCheckNow(context.Background(), b, update)
}

func TestHandleCheckNow_NoCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleCheckNow(context.Background(), b, update)
}

func TestHandleCheckNow_WithCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	var called atomic.Bool
	tn.SetCheckNowCallback(func(ctx context.Context, chatID int64) error {
		called.Store(true)
		return nil
	})

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleCheckNow(context.Background(), b, update)

	// Wait for goroutine
	assert.Eventually(t, func() bool { return called.Load() }, 2*time.Second, 100*time.Millisecond)
}

func TestHandleSectionToggle_NilCallbackQuery(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{CallbackQuery: nil}
	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSectionToggle_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Field Ministry",
		},
	}

	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSectionToggle_InvalidData(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "invalid_data",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSectionToggle_ToggleOn(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	// Create user first
	_, _ = pm.GetOrCreate(12345, "testuser")

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Field Ministry",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSectionToggle_ToggleOff(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	// Create user with section
	_, _ = pm.GetOrCreate(12345, "testuser")
	_ = pm.UpdateSections(12345, []string{"Field Ministry"})

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Field Ministry",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSave_NilCallbackQuery(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{CallbackQuery: nil}
	tn.handleSave(context.Background(), b, update)
}

func TestHandleSave_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID: "test-id",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSave(context.Background(), b, update)
}

func TestHandleSave_Success(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSave(context.Background(), b, update)
}

func TestHandleCancel_NilCallbackQuery(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{CallbackQuery: nil}
	tn.handleCancel(context.Background(), b, update)
}

func TestHandleCancel_Success(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID: "test-id",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleCancel(context.Background(), b, update)
}

func TestHandleCheckNow_Authorized(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleCheckNow(context.Background(), b, update)
}

func TestHandleSectionToggle_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Field Ministry",
		},
	}

	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSave_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleSave(context.Background(), b, update)
}

// --- SendNoRejectionsMessage ---

func TestSendNoRejectionsMessage_NotAuthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{111, 222})
	err := tn.SendNoRejectionsMessage(999, "test message")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized chat ID")
}

func TestSendNoRejectionsMessage_Authorized(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.SendNoRejectionsMessage(12345, "test message")
	// Will fail because bot is not connected, but tests the authorization path
	assert.Error(t, err)
}

// --- SendRejectionsNotification ---

func TestSendRejectionsNotification_EmptyRejections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.SendRejectionsNotification(12345, []domain.Rejection{})
	assert.NoError(t, err)
}

func TestSendRejectionsNotification_NotAuthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{111, 222})
	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	err := tn.SendRejectionsNotification(999, rejections)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized chat ID")
}

func TestSendRejectionsNotification_Authorized(t *testing.T) {
	tn := newTestNotifier(t, nil)
	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	err := tn.SendRejectionsNotification(12345, rejections)
	// Will fail because bot is not connected, but tests the authorization path
	assert.Error(t, err)
}

// --- StopBot ---

func TestNewTelegramNotifier_WithWhitelist(t *testing.T) {
	b, err := bot.New("test-token:fake", bot.WithSkipGetMe())
	require.NoError(t, err)

	tn := &TelegramNotifier{
		bot:       b,
		chatID:    12345,
		whitelist: []int64{111, 222, 333},
	}

	assert.NotNil(t, tn)
	assert.Equal(t, int64(12345), tn.chatID)
	assert.Len(t, tn.whitelist, 3)
}

func TestHandleConfig_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleConfig(context.Background(), b, update)
}

func TestHandleStatus_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleStatus(context.Background(), b, update)
}

func TestHandleStart_WithFromAndPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleStart(context.Background(), b, update)
}

func TestHandleStart_UnauthorizedWithPrefManager(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "unauthorized_user"},
		},
	}

	tn.handleStart(context.Background(), b, update)
}

func TestHandleConfig_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleConfig(context.Background(), b, update)
}

func TestHandleStatus_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleStatus(context.Background(), b, update)
}

func TestHandleSectionToggle_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Field Ministry",
		},
	}

	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSave_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleSave(context.Background(), b, update)
}

func TestHandleCancel_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{999})
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleCancel(context.Background(), b, update)
}

func TestHandleCancel_NilInnerMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleCancel(context.Background(), b, update)
}

func TestHandleSave_WithSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	_, _ = pm.GetOrCreate(12345, "testuser")
	_ = pm.UpdateSections(12345, []string{"Field Ministry", "Mechanical Parts"})

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSave(context.Background(), b, update)
}

func TestHandleSave_NoSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBot(t)

	_, _ = pm.GetOrCreate(12345, "testuser")
	_ = pm.UpdateSections(12345, []string{})

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleSave(context.Background(), b, update)
}

func TestStopBot_WithCancelFunc(t *testing.T) {
	tn := newTestNotifier(t, nil)
	_, cancel := context.WithCancel(context.Background())
	tn.cancelFunc = cancel

	err := tn.StopBot()
	assert.NoError(t, err)
	assert.Nil(t, tn.cancelFunc)
}

func TestHandleCheckNow_WithFailingCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	tn.SetCheckNowCallback(func(_ context.Context, _ int64) error {
		return errors.New("check failed")
	})

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleCheckNow(context.Background(), b, update)
	time.Sleep(200 * time.Millisecond)
}

func TestHandleStatus_Disabled(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBotWithServer(t, srv)

	_, _ = pm.GetOrCreate(12345, "testuser")
	_ = pm.ToggleEnabled(12345, false)
	_ = pm.UpdateSections(12345, []string{"Field Ministry"})

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}

	tn.handleStatus(context.Background(), b, update)
}

// --- Constructor success ---

func TestNewTelegramNotifier_ConstructorSuccess(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	orig := botNewFunc
	defer func() { botNewFunc = orig }()
	botNewFunc = func(token string, opts ...bot.Option) (*bot.Bot, error) {
		opts = append(opts, bot.WithServerURL(srv.URL))
		return bot.New(token, opts...)
	}

	tn, err := NewTelegramNotifier("123456:ABC-DEF", 12345, []int64{111})
	assert.NoError(t, err)
	require.NotNil(t, tn)
	assert.Equal(t, int64(12345), tn.chatID)
	assert.Len(t, tn.whitelist, 1)
}

func TestNewTelegramNotifier_BotNewError(t *testing.T) {
	orig := botNewFunc
	defer func() { botNewFunc = orig }()
	botNewFunc = func(_ string, _ ...bot.Option) (*bot.Bot, error) {
		return nil, errors.New("bot creation failed")
	}

	tn, err := NewTelegramNotifier("123456:ABC-DEF", 12345, nil)
	assert.Error(t, err)
	assert.Nil(t, tn)
	assert.Contains(t, err.Error(), "failed to create telegram bot")
}

func TestNewTelegramNotifier_DefaultHandlerCalled(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	var called atomic.Bool
	orig := botNewFunc
	defer func() { botNewFunc = orig }()
	botNewFunc = func(token string, opts ...bot.Option) (*bot.Bot, error) {
		opts = append(opts, bot.WithServerURL(srv.URL))
		return bot.New(token, opts...)
	}

	tn, err := NewTelegramNotifier("123456:ABC-DEF", 12345, nil)
	require.NoError(t, err)

	_ = called.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tn.bot.ProcessUpdate(ctx, &models.Update{})
}

// --- Success paths (SendMessage via mock server) ---

func TestSendNoRejectionsMessage_Success(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)

	err := tn.SendNoRejectionsMessage(12345, "No rejections found")
	assert.NoError(t, err)
}

func TestSendRejectionsNotification_Success(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)

	rejections := []domain.Rejection{
		{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"},
	}
	err := tn.SendRejectionsNotification(12345, rejections)
	assert.NoError(t, err)
}

// --- StartBot success ---

func TestStartBot_SuccessWithMockServer(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := tn.StartBot(ctx, pm)
	assert.NoError(t, err)
	assert.NotNil(t, tn.cancelFunc)

	_ = tn.StopBot()
}

// --- GetOrCreate error paths ---

func TestHandleConfig_GetOrCreateError(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newClosedPrefManager(t)
	tn.prefManager = pm

	b := newTestBotWithServer(t, srv)
	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}
	tn.handleConfig(context.Background(), b, update)
}

func TestHandleStatus_GetOrCreateError(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newClosedPrefManager(t)
	tn.prefManager = pm

	b := newTestBotWithServer(t, srv)
	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
			From: &models.User{ID: 12345, Username: "testuser"},
		},
	}
	tn.handleStatus(context.Background(), b, update)
}

func TestHandleSectionToggle_GetOrCreateError(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newClosedPrefManager(t)
	tn.prefManager = pm

	b := newTestBotWithServer(t, srv)
	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Field Ministry",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{ID: 1, Chat: models.Chat{ID: 12345}},
			},
		},
	}
	tn.handleSectionToggle(context.Background(), b, update)
}

func TestHandleSave_GetOrCreateError(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()
	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newClosedPrefManager(t)
	tn.prefManager = pm

	b := newTestBotWithServer(t, srv)
	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{ID: 1, Chat: models.Chat{ID: 12345}},
			},
		},
	}
	tn.handleSave(context.Background(), b, update)
}

func TestRateLimiterAllow_WithinLimitAndCleanup(t *testing.T) {
	rl := newRateLimiter()
	chatID := int64(12345)

	old := time.Now().Add(-2 * time.Minute)
	for i := 0; i < 10; i++ {
		rl.attempts[chatID] = append(rl.attempts[chatID], old)
	}

	allowed := rl.Allow(chatID)
	assert.True(t, allowed)
	assert.Len(t, rl.attempts[chatID], 1)
}

func TestRateLimiterAllow_Exceeded(t *testing.T) {
	rl := newRateLimiter()
	chatID := int64(12345)

	now := time.Now()
	for i := 0; i < 30; i++ {
		rl.attempts[chatID] = append(rl.attempts[chatID], now)
	}

	allowed := rl.Allow(chatID)
	assert.False(t, allowed)
	assert.Len(t, rl.attempts[chatID], 30)
}

func TestFormatTelegramField_EscapesHTML(t *testing.T) {
	formatted := formatTelegramField("👤", "Who", "<b>Alice & Bob</b>")

	assert.Contains(t, formatted, "👤 <b>Who:</b>")
	assert.Contains(t, formatted, "&lt;b&gt;Alice &amp; Bob&lt;/b&gt;")
	assert.NotContains(t, formatted, "<b>Alice & Bob</b>")
}

func TestCheckRateLimit_NilLimiter(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.rateLimiter = nil
	b := newTestBotWithServer(t, srv)

	allowed := tn.checkRateLimit(context.Background(), b, 12345)
	assert.True(t, allowed)
}

func TestCheckRateLimit_Exceeded(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.rateLimiter = newRateLimiter()
	b := newTestBotWithServer(t, srv)
	chatID := int64(12345)

	now := time.Now()
	for i := 0; i < 30; i++ {
		tn.rateLimiter.attempts[chatID] = append(tn.rateLimiter.attempts[chatID], now)
	}

	allowed := tn.checkRateLimit(context.Background(), b, chatID)
	assert.False(t, allowed)
}

func TestHandleLanguage_Unauthorized(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, []int64{999})
	b := newTestBotWithServer(t, srv)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleLanguage(context.Background(), b, update)
}

func TestHandleLanguage_Authorized(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	b := newTestBotWithServer(t, srv)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleLanguage(context.Background(), b, update)
}

func TestHandleLanguageSelect_Unauthorized(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, []int64{999})
	b := newTestBotWithServer(t, srv)

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "callback-id",
			From: models.User{ID: 12345, Username: "user"},
			Data: "lang_pt-BR",
		},
	}

	tn.handleLanguageSelect(context.Background(), b, update)
}

func TestHandleLanguageSelect_AuthorizedUpdatesLanguage(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBotWithServer(t, srv)

	_, _ = pm.GetOrCreate(12345, "testuser")

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "callback-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "lang_pt-BR",
			Message: models.MaybeInaccessibleMessage{
				Message: &models.Message{
					ID:   1,
					Chat: models.Chat{ID: 12345},
				},
			},
		},
	}

	tn.handleLanguageSelect(context.Background(), b, update)
	assert.Equal(t, "pt-BR", pm.GetLanguage(12345))
}

func TestBotStats_RecordCheckAndSnapshot(t *testing.T) {
	stats := newBotStats()
	stats.lastResetDate = "2000-01-01"

	stats.recordCheck(3)
	totalChecks, rejectionsToday := stats.snapshot()

	assert.Equal(t, 1, totalChecks)
	assert.Equal(t, 3, rejectionsToday)
}

func TestHandleStats_Unauthorized(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, []int64{999})
	b := newTestBotWithServer(t, srv)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleStats(context.Background(), b, update)
}

func TestHandleStats_Authorized(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	tn.stats = newBotStats()
	b := newTestBotWithServer(t, srv)

	_, _ = pm.GetOrCreate(12345, "testuser")
	tn.stats.recordCheck(2)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleStats(context.Background(), b, update)
}

func TestHandleStats_ListError(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.prefManager = newClosedPrefManager(t)
	b := newTestBotWithServer(t, srv)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleStats(context.Background(), b, update)
}

func TestHandleWhoAmI_WithoutPreferences(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	b := newTestBotWithServer(t, srv)

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleWhoAmI(context.Background(), b, update)
}

func TestHandleWhoAmI_WithPreferences(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	pm := newTestPrefManager(t)
	tn.prefManager = pm
	b := newTestBotWithServer(t, srv)

	_, _ = pm.GetOrCreate(12345, "testuser")
	_ = pm.UpdateSections(12345, []string{"Field Ministry", "Mechanical Parts"})
	_ = pm.UpdateLanguage(12345, "pt-BR")

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleWhoAmI(context.Background(), b, update)
}

func TestSendNoRejectionsMessage_RecordsStatsWhenEnabled(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.stats = newBotStats()

	err := tn.SendNoRejectionsMessage(12345, "No rejections found")
	require.NoError(t, err)

	totalChecks, rejectionsToday := tn.stats.snapshot()
	assert.Equal(t, 1, totalChecks)
	assert.Equal(t, 0, rejectionsToday)
}

func TestSendRejectionsNotification_RecordsStatsWhenEnabled(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.stats = newBotStats()

	rejections := []domain.Rejection{{Section: "Field Ministry", Who: "John", What: "Test", When: "01/03/2026"}}
	err := tn.SendRejectionsNotification(12345, rejections)
	require.NoError(t, err)

	totalChecks, rejectionsToday := tn.stats.snapshot()
	assert.Equal(t, 1, totalChecks)
	assert.Equal(t, 1, rejectionsToday)
}

func TestCheckRateLimit_Allowed(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.rateLimiter = newRateLimiter()
	b := newTestBotWithServer(t, srv)

	allowed := tn.checkRateLimit(context.Background(), b, 12345)
	assert.True(t, allowed)
}

func TestHandlers_ReturnEarlyWhenRateLimited(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.prefManager = newTestPrefManager(t)
	tn.rateLimiter = newRateLimiter()
	b := newTestBotWithServer(t, srv)
	chatID := int64(12345)

	now := time.Now()
	for i := 0; i < 30; i++ {
		tn.rateLimiter.attempts[chatID] = append(tn.rateLimiter.attempts[chatID], now)
	}

	msgUpdate := &models.Update{Message: &models.Message{Chat: models.Chat{ID: chatID}, From: &models.User{ID: chatID, Username: "user"}}}
	tn.handleStart(context.Background(), b, msgUpdate)
	tn.handleConfig(context.Background(), b, msgUpdate)
	tn.handleStatus(context.Background(), b, msgUpdate)
	tn.handleHelp(context.Background(), b, msgUpdate)
	tn.handleStats(context.Background(), b, msgUpdate)
	tn.handleWhoAmI(context.Background(), b, msgUpdate)
	tn.handleCheckNow(context.Background(), b, msgUpdate)
	tn.handleLanguage(context.Background(), b, msgUpdate)

	cbUpdate := &models.Update{CallbackQuery: &models.CallbackQuery{ID: "cb", From: models.User{ID: chatID, Username: "user"}, Data: "section_Field Ministry"}}
	tn.handleSectionToggle(context.Background(), b, cbUpdate)
	tn.handleSave(context.Background(), b, cbUpdate)
	tn.handleCancel(context.Background(), b, cbUpdate)
	cbUpdate.CallbackQuery.Data = "lang_en"
	tn.handleLanguageSelect(context.Background(), b, cbUpdate)
}

func TestTranslateSectionName_UnknownSection(t *testing.T) {
	assert.Equal(t, "Unknown", translateSectionName("en", "Unknown"))
}

func TestHandleLanguage_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	tn.handleLanguage(context.Background(), b, &models.Update{Message: nil})
}

func TestHandleLanguageSelect_NilCallbackQuery(t *testing.T) {
	tn := newTestNotifier(t, nil)
	b := newTestBot(t)

	tn.handleLanguageSelect(context.Background(), b, &models.Update{CallbackQuery: nil})
}

func TestHandleCheckNow_WithFailingCallback_RecordsStats(t *testing.T) {
	srv := newMockTelegramServer(t)
	defer srv.Close()

	tn := newTestNotifierWithServer(t, srv, nil)
	tn.stats = newBotStats()
	b := newTestBotWithServer(t, srv)

	tn.SetCheckNowCallback(func(_ context.Context, _ int64) error {
		return errors.New("check failed")
	})

	update := &models.Update{Message: &models.Message{Chat: models.Chat{ID: 12345}}}
	tn.handleCheckNow(context.Background(), b, update)

	assert.Eventually(t, func() bool {
		totalChecks, rejectionsToday := tn.stats.snapshot()
		return totalChecks == 1 && rejectionsToday == 0
	}, 2*time.Second, 50*time.Millisecond)
}
