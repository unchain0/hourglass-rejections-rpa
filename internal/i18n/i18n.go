// Package i18n provides internationalization support for the application.
package i18n

import (
	"embed"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localeFS embed.FS

var bundle *i18n.Bundle

// Init initializes the i18n bundle with all supported languages.
func Init() error {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	if _, err := bundle.LoadMessageFileFS(localeFS, "locales/en.toml"); err != nil {
		return fmt.Errorf("failed to load English translations: %w", err)
	}
	if _, err := bundle.LoadMessageFileFS(localeFS, "locales/pt-BR.toml"); err != nil {
		return fmt.Errorf("failed to load Portuguese translations: %w", err)
	}
	if _, err := bundle.LoadMessageFileFS(localeFS, "locales/es.toml"); err != nil {
		return fmt.Errorf("failed to load Spanish translations: %w", err)
	}
	if _, err := bundle.LoadMessageFileFS(localeFS, "locales/fr.toml"); err != nil {
		return fmt.Errorf("failed to load French translations: %w", err)
	}

	return nil
}

// GetLocalizer returns a localizer for the given language tag.
func GetLocalizer(langTag string) *i18n.Localizer {
	if langTag == "" {
		return i18n.NewLocalizer(bundle, "en")
	}
	return i18n.NewLocalizer(bundle, langTag)
}

// Localize translates a message ID to the specified language.
func Localize(lang string, messageID string, templateData map[string]interface{}) string {
	localizer := GetLocalizer(lang)
	return localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
}
