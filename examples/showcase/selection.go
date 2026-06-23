package main

import (
	"net/http"

	partial "github.com/donseba/go-partial"
)

func (app *App) selection(w http.ResponseWriter, r *http.Request) {
	summary := partial.NewID("summary", "templates/selection_summary.gohtml").SetData(map[string]any{
		"Title": "Summary",
	})
	details := partial.NewID("details", "templates/selection_details.gohtml").SetData(map[string]any{
		"Title": "Details",
	})
	content := partial.NewID("content", "templates/selection.gohtml").SetData(map[string]any{
		"Title": "Selection partials",
	})
	content.WithSelectMap("summary", map[string]*partial.Partial{
		"summary": summary,
		"details": details,
	})
	app.renderPartial(w, r, content)
}
