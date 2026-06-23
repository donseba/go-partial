package main

import (
	"net/http"

	"github.com/donseba/go-partial/connector"
)

func (app *App) action(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.Header.Get(connector.HeaderAction.String()) == "increment" {
		app.counter++
	}
	app.render(w, r, "content", "templates/action.gohtml", map[string]any{
		"Title":        "Action callbacks",
		"Counter":      app.counter,
		"ActionHeader": connector.HeaderAction.String(),
	})
}
