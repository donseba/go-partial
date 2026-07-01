package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/target"
)

func (app *App) rowsPage(w http.ResponseWriter, r *http.Request) {
	content := app.tablePartial()
	app.renderPartial(w, r, content)
}

func (app *App) tablePartial() *partial.Partial {
	rowPartial := partial.NewID("row", "templates/row.gohtml")
	content := partial.NewID("content", "templates/rows.gohtml").SetDot(RowsPage{
		Title: "Typed rows",
		Rows:  app.rows,
	})
	content.With(rowPartial)
	target.WithResolver(content, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
		if !strings.HasPrefix(target, "row-") {
			return nil, false
		}
		id, err := strconv.Atoi(strings.TrimPrefix(target, "row-"))
		if err != nil {
			return nil, false
		}
		for _, row := range app.rows {
			if row.ID == id {
				row.Status = "Updated " + time.Now().Format("15:04:05")
				return partial.NewID(target, "templates/row.gohtml").SetDot(row), true
			}
		}
		return nil, false
	})
	return content
}

func (app *App) refreshRow(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("id")
	app.writeContent(w, r, app.tablePartial())
}
