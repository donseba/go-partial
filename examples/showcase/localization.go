package main

import "net/http"

func (app *App) localization(w http.ResponseWriter, r *http.Request) {
	locale := app.localeFromRequest(r)
	app.render(w, r, "content", "templates/localization.gohtml", LocalizationPage{
		Title:   "Localization",
		Locale:  locale,
		Locales: []string{"en_US", "nl_NL", "fr_FR"},
		Count:   5,
	})
}
