package main

import (
	"net/http"

	partial "github.com/donseba/go-partial"
)

func (app *App) errorPage(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/error.gohtml").SetData(map[string]any{
		"Title": "Template error boundary",
	})
	content.With(partial.NewID("broken-section", "templates/broken.gohtml"))
	app.renderPartial(w, r, content)
}
