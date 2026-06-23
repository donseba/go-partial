package main

import (
	"net/http"

	partial "github.com/donseba/go-partial"
)

func (app *App) contextPage(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/context.gohtml").SetData(map[string]any{
		"Title": "Context helpers",
	})
	app.renderPartial(w, r, content)
}
