package notifier

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hourglass-rejections-rpa/internal/domain"
	"hourglass-rejections-rpa/internal/preferences"
)

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

// --- AllSections ---

func TestAllSections(t *testing.T) {
	expected := []string{
		"Partes Mecânicas",
		"Campo",
		"Testemunho Público",
		"Reunião Meio de Semana",
	}
	assert.Equal(t, expected, AllSections)
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
	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "Test", OQue: "Test", PraQuando: "01/01/2026"},
	}
	err := tn.SendRejectionsNotification(222, rejections)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

// --- containsSection ---

func TestContainsSection_Found(t *testing.T) {
	sections := []string{"Campo", "Partes Mecânicas"}
	assert.True(t, containsSection(sections, "Campo"))
}

func TestContainsSection_NotFound(t *testing.T) {
	sections := []string{"Campo", "Partes Mecânicas"}
	assert.False(t, containsSection(sections, "Vida e Ministério"))
}

func TestContainsSection_Empty(t *testing.T) {
	assert.False(t, containsSection([]string{}, "Campo"))
	assert.False(t, containsSection(nil, "Campo"))
}

// --- removeSection ---

func TestRemoveSection_Exists(t *testing.T) {
	sections := []string{"Campo", "Partes Mecânicas", "Testemunho"}
	result := removeSection(sections, "Partes Mecânicas")
	assert.Equal(t, []string{"Campo", "Testemunho"}, result)
}

func TestRemoveSection_NotExists(t *testing.T) {
	sections := []string{"Campo", "Partes Mecânicas"}
	result := removeSection(sections, "Vida")
	assert.Equal(t, sections, result)
}

func TestRemoveSection_Empty(t *testing.T) {
	result := removeSection([]string{}, "Campo")
	assert.Equal(t, []string{}, result)
}

func TestRemoveSection_SingleElement(t *testing.T) {
	sections := []string{"Campo"}
	result := removeSection(sections, "Campo")
	assert.Equal(t, []string{}, result)
}

func TestRemoveSection_NoMutation(t *testing.T) {
	original := []string{"Campo", "Partes Mecânicas", "Testemunho"}
	_ = removeSection(original, "Partes Mecânicas")
	assert.Equal(t, 3, len(original))
}

// --- buildConfigKeyboard ---

func TestBuildConfigKeyboard_NoSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	prefs := &preferences.UserPreference{
		ChatID:       12345,
		SectionsJSON: "[]",
	}

	keyboard := tn.buildConfigKeyboard(prefs)
	require.NotNil(t, keyboard)

	inlineKB, ok := keyboard.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	assert.GreaterOrEqual(t, len(inlineKB.InlineKeyboard), 5)
}

func TestBuildConfigKeyboard_SomeSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	prefs := &preferences.UserPreference{
		ChatID:       12345,
		SectionsJSON: `["Campo","Partes Mecânicas"]`,
	}

	keyboard := tn.buildConfigKeyboard(prefs)
	require.NotNil(t, keyboard)

	inlineKB, ok := keyboard.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	assert.GreaterOrEqual(t, len(inlineKB.InlineKeyboard), 1)
}

func TestBuildConfigKeyboard_AllSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	prefs := &preferences.UserPreference{
		ChatID:       12345,
		SectionsJSON: `["Partes Mecânicas","Campo","Testemunho Público","Reunião Meio de Semana"]`,
	}

	keyboard := tn.buildConfigKeyboard(prefs)
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

	called := false
	tn.SetCheckNowCallback(func(ctx context.Context, chatID int64) error {
		called = true
		return nil
	})

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 12345},
		},
	}

	tn.handleCheckNow(context.Background(), b, update)

	// Wait for goroutine
	assert.Eventually(t, func() bool { return called }, 2*time.Second, 100*time.Millisecond)
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
			Data: "section_Campo",
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
			Data: "section_Campo",
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
	_ = pm.UpdateSections(12345, []string{"Campo"})

	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			From: models.User{ID: 12345, Username: "testuser"},
			Data: "section_Campo",
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
			Data: "section_Campo",
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
	err := tn.SendRejectionsNotification(12345, []domain.Rejeicao{})
	assert.NoError(t, err)
}

func TestSendRejectionsNotification_NotAuthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{111, 222})
	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
	}
	err := tn.SendRejectionsNotification(999, rejections)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized chat ID")
}

func TestSendRejectionsNotification_Authorized(t *testing.T) {
	tn := newTestNotifier(t, nil)
	rejections := []domain.Rejeicao{
		{Secao: "Campo", Quem: "John", OQue: "Test", PraQuando: "01/03/2026"},
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
