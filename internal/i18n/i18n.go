// Package i18n provides internationalization support for the application.
package i18n

import (
	"embed"
	"fmt"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localeFS embed.FS

var bundle *i18n.Bundle
var initOnce sync.Once
var initErr error

var loadMessageFile = func(path string) error {
	_, err := bundle.LoadMessageFileFS(localeFS, path)
	return err
}

// Init initializes the i18n bundle with all supported languages.
func Init() error {
	initOnce.Do(func() {
		bundle = i18n.NewBundle(language.English)
		bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

		if err := loadMessageFile("locales/en.toml"); err != nil {
			initErr = fmt.Errorf("failed to load English translations: %w", err)
			return
		}
		if err := loadMessageFile("locales/pt-BR.toml"); err != nil {
			initErr = fmt.Errorf("failed to load Portuguese translations: %w", err)
			return
		}
		if err := loadMessageFile("locales/es.toml"); err != nil {
			initErr = fmt.Errorf("failed to load Spanish translations: %w", err)
			return
		}
		if err := loadMessageFile("locales/fr.toml"); err != nil {
			initErr = fmt.Errorf("failed to load French translations: %w", err)
			return
		}
	})

	return initErr
}

func resetForTesting() {
	initOnce = sync.Once{}
	initErr = nil
	bundle = nil
}

// GetLocalizer returns a localizer for the given language tag.
func GetLocalizer(langTag string) *i18n.Localizer {
	if bundle == nil {
		_ = Init()
	}
	if langTag == "" {
		return i18n.NewLocalizer(bundle, "en")
	}
	return i18n.NewLocalizer(bundle, langTag)
}

// Localize translates a message ID to the specified language.
func Localize(lang string, messageID string, templateData map[string]interface{}) string {
	localizer := GetLocalizer(lang)
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	if err != nil {
		return messageID
	}
	return msg
}
