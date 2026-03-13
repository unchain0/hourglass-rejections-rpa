package i18n

import (
	"embed"
	"fmt"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
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

func GetLocalizer(langTag string) *i18n.Localizer {
	if bundle == nil {
		_ = Init()
	}
	if langTag == "" {
		return i18n.NewLocalizer(bundle, "en")
	}
	return i18n.NewLocalizer(bundle, langTag)
}

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

var dateFormats = map[string]string{
	"en":    "01/02/2006",
	"pt-BR": "02/01/2006",
	"es":    "02/01/2006",
	"fr":    "02/01/2006",
}

// FormatDate formats an ISO date string (YYYY-MM-DD) according to the specified language locale.
func FormatDate(isoDate string, lang string) string {
	t, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}

	format, ok := dateFormats[lang]
	if !ok {
		format = dateFormats["en"]
	}

	p := message.NewPrinter(language.MustParse(lang))
	return p.Sprintf("%s", t.Format(format))
}
