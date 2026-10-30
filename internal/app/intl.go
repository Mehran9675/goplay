package app

import (
	"embed"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/i18n"
)

// GoPlay's text is translated by its English: the English is the key
// into locales/<language>.json, and text a language lacks stays English.
// Placeholders are {name}, filled from name, value pairs; a "count"
// value picks a plural form (en.json holds the English plurals).

//go:embed locales
var locales embed.FS

// intlOptions loads the translations built into the executable.
var intlOptions = &giode.IntlOptions{FS: locales, Dir: "locales", Fallback: "en"}

// tr translates s, filling its {placeholders} from name, value pairs.
func tr(s string, data ...any) string { return giode.Intl(s, data...) }

// uiLocale is the locale for the language setting's value: the system's
// for "".
func uiLocale(lang string) string {
	if lang == "" {
		return i18n.SystemLocale()
	}
	return lang
}
