package main

import (
	"net/http"
)

func (app *App) oob(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/oob.gohtml", map[string]any{
		"Title": "Navigation jokes",
	})
}

func (app *App) oobPing(w http.ResponseWriter, r *http.Request) {
	app.oob(w, r)
}
