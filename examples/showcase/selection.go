package main

import (
	"net/http"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/selection"
)

func (app *App) selection(w http.ResponseWriter, r *http.Request) {
	summary := partial.NewID("summary", "templates/selection_summary.gohtml").SetDot(SelectionPanel{
		Title: "Summary",
	})
	details := partial.NewID("details", "templates/selection_details.gohtml").SetDot(SelectionPanel{
		Title: "Details",
	})
	content := partial.NewID("content", "templates/selection.gohtml").SetDot(PageTitle{
		Title: "Selection partials",
	})
	selection.WithSelectMap(content, "summary", map[string]*partial.Partial{
		"summary": summary,
		"details": details,
	})
	app.renderPartial(w, r, content)
}
