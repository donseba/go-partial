package main

import (
	"net/http"

	partial "github.com/donseba/go-partial"
)

func (app *App) tabs(w http.ResponseWriter, r *http.Request) {
	overview := partial.NewID("overview", "templates/tabs_overview.gohtml")
	activity := partial.NewID("activity", "templates/tabs_activity.gohtml")
	settings := partial.NewID("settings", "templates/tabs_settings.gohtml")
	failing := partial.NewID("failing", "templates/tabs_failing.gohtml")
	content := partial.NewID("content", "templates/tabs.gohtml").SetData(map[string]any{
		"Title": "Tabs with selection",
		"Tabs": []map[string]string{
			{"Key": "overview", "Label": "Overview"},
			{"Key": "activity", "Label": "Activity"},
			{"Key": "settings", "Label": "Settings"},
			{"Key": "failing", "Label": "Fails"},
		},
	})
	content.WithSelectMap("overview", map[string]*partial.Partial{
		"overview": overview,
		"activity": activity,
		"settings": settings,
		"failing":  failing,
	})
	app.renderPartial(w, r, content)
}
