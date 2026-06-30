package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/flash"
)

func (app *App) asyncPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/async.gohtml", AsyncPage{
		Title: "Deferred partials",
		Rows:  app.rows,
	})
}

func (app *App) asyncStats(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("async-stats", "templates/async_stats.gohtml").SetDot(AsyncStats{
		RenderedAt: time.Now().Format("15:04:05"),
		Rows:       len(app.rows),
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
			delay := time.Duration(row.ID*2) * time.Second
			time.Sleep(delay)
			content := partial.NewID("async-row", "templates/async_row.gohtml").SetDot(AsyncRow{
				Row:        row,
				RenderedAt: time.Now().Format("15:04:05"),
			})
			ctx := flash.Add(r.Context(), flash.Success(fmt.Sprintf("Row %d loaded after %s", row.ID, delay)))
			r = r.WithContext(ctx)
			app.writeStandalone(w, r, content)
			return
		}
	}
	http.NotFound(w, r)
}
