package main

import (
	"net/http"
)

func (app *App) home(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/home.gohtml", map[string]any{
		"Title": "Server-rendered partials",
	})
}
