package i18n

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_LoadErrors(t *testing.T) {
	tests := []struct {
		name        string
		failPath    string
		errorPrefix string
	}{
		{name: "english load failure", failPath: "locales/en.toml", errorPrefix: "failed to load English translations"},
		{name: "portuguese load failure", failPath: "locales/pt-BR.toml", errorPrefix: "failed to load Portuguese translations"},
		{name: "spanish load failure", failPath: "locales/es.toml", errorPrefix: "failed to load Spanish translations"},
		{name: "french load failure", failPath: "locales/fr.toml", errorPrefix: "failed to load French translations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLoader := loadMessageFile
			t.Cleanup(func() { loadMessageFile = originalLoader })

			loadMessageFile = func(path string) error {
				if path == tt.failPath {
					return errors.New("boom")
				}
				return originalLoader(path)
			}

			err := Init()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorPrefix)
			assert.Contains(t, err.Error(), "boom")
		})
	}
}

func TestInit(t *testing.T) {
	t.Run("successful initialization", func(t *testing.T) {
		err := Init()
		require.NoError(t, err)
		assert.NotNil(t, bundle)
	})

	t.Run("idempotent initialization", func(t *testing.T) {
		err := Init()
		require.NoError(t, err)
	})
}

func TestGetLocalizer(t *testing.T) {
	if bundle == nil {
		err := Init()
		require.NoError(t, err)
	}

	tests := []struct {
		name     string
		langTag  string
		wantLang string
	}{
		{
			name:     "English language",
			langTag:  "en",
			wantLang: "en",
		},
		{
			name:     "Portuguese language",
			langTag:  "pt-BR",
			wantLang: "pt-BR",
		},
		{
			name:     "Empty language defaults to English",
			langTag:  "",
			wantLang: "en",
		},
		{
			name:     "Spanish language",
			langTag:  "es",
			wantLang: "es",
		},
		{
			name:     "French language",
			langTag:  "fr",
			wantLang: "fr",
		},
		{
			name:     "Invalid language falls back",
			langTag:  "invalid",
			wantLang: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localizer := GetLocalizer(tt.langTag)
			assert.NotNil(t, localizer)
		})
	}
}

func TestLocalize(t *testing.T) {
	if bundle == nil {
		err := Init()
		require.NoError(t, err)
	}

	tests := []struct {
		name         string
		lang         string
		messageID    string
		templateData map[string]interface{}
		wantContain  string
	}{
		{
			name:         "English welcome message",
			lang:         "en",
			messageID:    "welcome",
			templateData: nil,
			wantContain:  "Welcome",
		},
		{
			name:         "Portuguese welcome message",
			lang:         "pt-BR",
			messageID:    "welcome",
			templateData: nil,
			wantContain:  "Bem-vindo",
		},
		{
			name:         "English with template data",
			lang:         "en",
			messageID:    "welcome_unauthorized",
			templateData: map[string]interface{}{"ChatID": "12345"},
			wantContain:  "12345",
		},
		{
			name:         "Portuguese with template data",
			lang:         "pt-BR",
			messageID:    "welcome_unauthorized",
			templateData: map[string]interface{}{"ChatID": "12345"},
			wantContain:  "12345",
		},
		{
			name:         "Default to English for unknown language",
			lang:         "unknown",
			messageID:    "welcome",
			templateData: nil,
			wantContain:  "Welcome",
		},
		{
			name:         "Empty language defaults to English",
			lang:         "",
			messageID:    "welcome",
			templateData: nil,
			wantContain:  "Welcome",
		},
		{
			name:         "Spanish welcome message",
			lang:         "es",
			messageID:    "welcome",
			templateData: nil,
			wantContain:  "Bienvenido",
		},
		{
			name:         "French welcome message",
			lang:         "fr",
			messageID:    "welcome",
			templateData: nil,
			wantContain:  "Bienvenue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Localize(tt.lang, tt.messageID, tt.templateData)
			assert.Contains(t, got, tt.wantContain)
		})
	}
}

func TestLocalize_AllKeys(t *testing.T) {
	if bundle == nil {
		err := Init()
		require.NoError(t, err)
	}

	keys := []string{
		"welcome",
		"welcome_unauthorized",
		"choose_sections",
		"your_preferences",
		"no_sections_selected",
		"language_select",
		"language_changed",
		"help_commands",
		"rate_limit_exceeded",
		"unauthorized",
		"configure_error",
		"preferences_saved",
		"check_now_requested",
		"check_now_unavailable",
		"no_rejections_found",
		"rejections_detected",
		"language_english",
		"language_portuguese",
		"language_spanish",
		"language_french",
		"btn_save",
		"btn_cancel",
		"configuration_cancelled",
		"stats_overview",
		"whoami_info",
		"whoami_no_sections",
		"status_yes",
		"status_no",
	}

	languages := []string{"en", "pt-BR", "es", "fr"}

	for _, lang := range languages {
		for _, key := range keys {
			t.Run(lang+"_"+key, func(t *testing.T) {
				result := Localize(lang, key, nil)
				assert.NotEmpty(t, result)
				assert.NotEqual(t, key, result)
			})
		}
	}
}

func TestLocalize_RejectionsTemplate(t *testing.T) {
	if bundle == nil {
		err := Init()
		require.NoError(t, err)
	}

	rejections := []map[string]interface{}{
		{
			"Number":  1,
			"Who":     "John",
			"Section": "Test Section",
			"What":    "Test Assignment",
			"When":    "2024-01-01",
		},
	}

	tests := []struct {
		name string
		lang string
	}{
		{name: "English rejections", lang: "en"},
		{name: "Portuguese rejections", lang: "pt-BR"},
		{name: "Spanish rejections", lang: "es"},
		{name: "French rejections", lang: "fr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Localize(tt.lang, "rejections_detected", map[string]interface{}{
				"Count":      1,
				"Rejections": rejections,
			})
			assert.NotEmpty(t, result)
			assert.Contains(t, result, "John")
			assert.Contains(t, result, "Test Section")
		})
	}
}
