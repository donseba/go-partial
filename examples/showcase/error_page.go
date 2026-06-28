package main

import (
	"net/http"

	partial "github.com/donseba/go-partial"
)

func (app *App) errorPage(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/error.gohtml").SetDot(PageTitle{
		Title: "Template error boundary",
	})
	app.renderPartial(w, r, content)
}

func (app *App) errorSection(w http.ResponseWriter, r *http.Request) {
	app.writeContent(w, r, partial.NewID("broken-section", "templates/broken.gohtml"))
}
