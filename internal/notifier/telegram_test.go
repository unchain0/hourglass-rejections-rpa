package notifier

import (
	"context"
	"path/filepath"
	"testing"

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

// newTestPrefManager creates a PreferenceManager backed by a temp file.
func newTestPrefManager(t *testing.T) *preferences.PreferenceManager {
	t.Helper()
	store := preferences.NewFilePreferenceStore(filepath.Join(t.TempDir(), "prefs.json"))
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
	assert.Len(t, AllSections, 4)
}

// --- NewTelegramNotifier ---

func TestNewTelegramNotifier_EmptyToken(t *testing.T) {
	_, err := NewTelegramNotifier("", 123, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram bot token is required")
}

func TestNewTelegramNotifier_ZeroChatID(t *testing.T) {
	_, err := NewTelegramNotifier("some-token", 0, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram chat ID is required")
}

// --- IsAuthorized ---

func TestIsAuthorized_EmptyWhitelist(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.True(t, tn.IsAuthorized(999))
}

func TestIsAuthorized_InWhitelist(t *testing.T) {
	tn := newTestNotifier(t, []int64{100, 200, 300})
	assert.True(t, tn.IsAuthorized(200))
}

func TestIsAuthorized_NotInWhitelist(t *testing.T) {
	tn := newTestNotifier(t, []int64{100, 200, 300})
	assert.False(t, tn.IsAuthorized(999))
}

// --- IsConfigured ---

func TestIsConfigured_Valid(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.True(t, tn.IsConfigured())
}

func TestIsConfigured_NilBot(t *testing.T) {
	tn := &TelegramNotifier{chatID: 123}
	assert.False(t, tn.IsConfigured())
}

func TestIsConfigured_ZeroChatID(t *testing.T) {
	tn := &TelegramNotifier{bot: newTestBot(t)}
	assert.False(t, tn.IsConfigured())
}

func TestIsConfigured_NilNotifier(t *testing.T) {
	var tn *TelegramNotifier
	assert.False(t, tn.IsConfigured())
}

// --- StartBot / StopBot ---

func TestStartBot_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.StartBot(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preference manager is required")
}

func TestStartBot_Success(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)

	err := tn.StartBot(context.Background(), pm)
	require.NoError(t, err)
	assert.NotNil(t, tn.prefManager)
	assert.NotNil(t, tn.cancelFunc)

	// StopBot should clean up
	err = tn.StopBot()
	require.NoError(t, err)
	assert.Nil(t, tn.cancelFunc)
}

func TestStopBot_NoCancelFunc(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.StopBot()
	require.NoError(t, err)
}

func TestStopBot_Idempotent(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pm := newTestPrefManager(t)

	err := tn.StartBot(context.Background(), pm)
	require.NoError(t, err)

	require.NoError(t, tn.StopBot())
	require.NoError(t, tn.StopBot()) // second call is safe
}

// --- containsSection ---

func TestContainsSection_Found(t *testing.T) {
	assert.True(t, containsSection([]string{"Campo", "Partes Mecânicas"}, "Campo"))
}

func TestContainsSection_NotFound(t *testing.T) {
	assert.False(t, containsSection([]string{"Campo", "Partes Mecânicas"}, "Testemunho Público"))
}

func TestContainsSection_Empty(t *testing.T) {
	assert.False(t, containsSection([]string{}, "Campo"))
}

func TestContainsSection_Nil(t *testing.T) {
	assert.False(t, containsSection(nil, "Campo"))
}

// --- removeSection ---

func TestRemoveSection_Exists(t *testing.T) {
	result := removeSection([]string{"Campo", "Partes Mecânicas", "Testemunho Público"}, "Partes Mecânicas")
	assert.Equal(t, []string{"Campo", "Testemunho Público"}, result)
}

func TestRemoveSection_NotExists(t *testing.T) {
	input := []string{"Campo", "Partes Mecânicas"}
	result := removeSection(input, "Testemunho Público")
	assert.Equal(t, []string{"Campo", "Partes Mecânicas"}, result)
}

func TestRemoveSection_Empty(t *testing.T) {
	result := removeSection([]string{}, "Campo")
	assert.Empty(t, result)
}

func TestRemoveSection_SingleElement(t *testing.T) {
	result := removeSection([]string{"Campo"}, "Campo")
	assert.Empty(t, result)
}

func TestRemoveSection_DoesNotMutateOriginal(t *testing.T) {
	original := []string{"Campo", "Partes Mecânicas"}
	_ = removeSection(original, "Campo")
	assert.Len(t, original, 2) // original slice is not mutated
}

// --- buildConfigKeyboard ---

func TestBuildConfigKeyboard_NoSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pref := &preferences.UserPreference{
		Sections: []string{},
	}

	kb := tn.buildConfigKeyboard(pref)
	markup, ok := kb.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	// 4 section rows + 1 save/cancel row
	require.Len(t, markup.InlineKeyboard, 5)

	// All sections should show ❌
	for i, section := range AllSections {
		row := markup.InlineKeyboard[i]
		require.Len(t, row, 1)
		assert.Equal(t, "❌ "+section, row[0].Text)
		assert.Equal(t, "section_"+section, row[0].CallbackData)
	}

	// Last row: Save and Cancel
	lastRow := markup.InlineKeyboard[4]
	require.Len(t, lastRow, 2)
	assert.Equal(t, "💾 Salvar", lastRow[0].Text)
	assert.Equal(t, "save_config", lastRow[0].CallbackData)
	assert.Equal(t, "🚫 Cancelar", lastRow[1].Text)
	assert.Equal(t, "cancel_config", lastRow[1].CallbackData)
}

func TestBuildConfigKeyboard_SomeSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pref := &preferences.UserPreference{
		Sections: []string{"Campo", "Reunião Meio de Semana"},
	}

	kb := tn.buildConfigKeyboard(pref)
	markup, ok := kb.(*models.InlineKeyboardMarkup)
	require.True(t, ok)
	require.Len(t, markup.InlineKeyboard, 5)

	// Check correct toggle indicators
	assert.Equal(t, "❌ Partes Mecânicas", markup.InlineKeyboard[0][0].Text)
	assert.Equal(t, "✅ Campo", markup.InlineKeyboard[1][0].Text)
	assert.Equal(t, "❌ Testemunho Público", markup.InlineKeyboard[2][0].Text)
	assert.Equal(t, "✅ Reunião Meio de Semana", markup.InlineKeyboard[3][0].Text)
}

func TestBuildConfigKeyboard_AllSections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	pref := &preferences.UserPreference{
		Sections: AllSections,
	}

	kb := tn.buildConfigKeyboard(pref)
	markup, ok := kb.(*models.InlineKeyboardMarkup)
	require.True(t, ok)

	// All should show ✅
	for i, section := range AllSections {
		assert.Equal(t, "✅ "+section, markup.InlineKeyboard[i][0].Text)
	}
}

// --- SendRejectionsNotification ---

func TestSendRejectionsNotification_EmptyRejections(t *testing.T) {
	tn := newTestNotifier(t, nil)
	err := tn.SendRejectionsNotification(12345, nil)
	assert.NoError(t, err)
}

func TestSendRejectionsNotification_Unauthorized(t *testing.T) {
	tn := newTestNotifier(t, []int64{100})
	err := tn.SendRejectionsNotification(999, []domain.Rejeicao{{Quem: "test"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized chat ID")
}

// --- Handler nil-safety ---

func TestHandleStart_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	// Should not panic with nil Message
	assert.NotPanics(t, func() {
		tn.handleStart(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleConfig_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleConfig(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleStatus_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleStatus(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleHelp_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleHelp(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleCheckNow_NilMessage(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleCheckNow(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleSectionToggle_NilCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleSectionToggle(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleSave_NilCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleSave(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleCancel_NilCallback(t *testing.T) {
	tn := newTestNotifier(t, nil)
	assert.NotPanics(t, func() {
		tn.handleCancel(context.Background(), tn.bot, &models.Update{})
	})
}

func TestHandleSave_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	// With a callback query but no prefManager, should not panic
	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			Data: "save_config",
			From: models.User{ID: 123},
		},
	}
	assert.NotPanics(t, func() {
		tn.handleSave(context.Background(), tn.bot, update)
	})
}

func TestHandleSectionToggle_NilPrefManager(t *testing.T) {
	tn := newTestNotifier(t, nil)
	update := &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "test-id",
			Data: "section_Campo",
			From: models.User{ID: 123},
		},
	}
	assert.NotPanics(t, func() {
		tn.handleSectionToggle(context.Background(), tn.bot, update)
	})
}
