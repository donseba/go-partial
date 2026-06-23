package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
)

func (app *App) asyncPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/async.gohtml", map[string]any{
		"Title": "Deferred partials",
		"Rows":  app.rows,
	})
}

func (app *App) asyncStats(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("async-stats", "templates/async_stats.gohtml").SetData(map[string]any{
		"RenderedAt": time.Now().Format("15:04:05"),
		"Rows":       len(app.rows),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) asyncRow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/async/row/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	for _, row := range app.rows {
		if row.ID == id {
			time.Sleep(time.Duration(row.ID*2) * time.Second)
			content := partial.NewID("async-row", "templates/async_row.gohtml").SetData(map[string]any{
				"Row":        row,
				"RenderedAt": time.Now().Format("15:04:05"),
			})
			app.writeStandalone(w, r, content)
			return
		}
	}
	http.NotFound(w, r)
}
