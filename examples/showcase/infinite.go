package main

import (
	"net/http"
	"strconv"
	"strings"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func (app *App) infinite(w http.ResponseWriter, r *http.Request) {
	content := app.infinitePartial("content", 1, 25)
	app.renderPartial(w, r, content)
}

func (app *App) infiniteLoad(w http.ResponseWriter, r *http.Request) {
	action := r.Header.Get(connector.HeaderAction.String())
	if !strings.HasPrefix(action, "current-") {
		http.Error(w, "missing X-Action: current-<row>", http.StatusBadRequest)
		return
	}

	current, err := strconv.Atoi(strings.TrimPrefix(action, "current-"))
	if err != nil || current < 0 {
		http.Error(w, "invalid X-Action cursor", http.StatusBadRequest)
		return
	}

	if current >= 150 {
		current = 125
	}

	start := current + 1
	content := app.infinitePartial("infinite-chunk", start, 25)
	app.writeStandalone(w, r, content)
}

func (app *App) infinitePartial(id string, start int, count int) *partial.Partial {
	end := start + count - 1
	if end > 150 {
		end = 150
	}

	rows := make([]int, 0, max(0, end-start+1))
	for i := start; i <= end; i++ {
		rows = append(rows, i)
	}

	templateName := "templates/infinite_chunk.gohtml"
	if id == "content" {
		templateName = "templates/infinite.gohtml"
	}

	content := partial.NewID(id, templateName).SetData(map[string]any{
		"Title":        "Infinite scroll with X-Action",
		"Rows":         rows,
		"Next":         end,
		"Done":         end >= 150,
		"Start":        start,
		"Current":      start - 1,
		"ActionHeader": connector.HeaderAction.String(),
	})
	content.With(partial.NewID("infinite-row", "templates/infinite_row.gohtml"))
	content.With(partial.NewID("rickroll", "templates/rickroll.gohtml"))
	content.With(partial.NewID("infinite-toast", "templates/infinite_toast.gohtml"))
	return content
}
