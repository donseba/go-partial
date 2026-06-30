package main

import (
	"net/http"
	"os"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/sse"
)

func (app *App) sse(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/sse.gohtml", PageTitle{
		Title: "Server-sent events",
	})
}

func (app *App) sseStream(w http.ResponseWriter, r *http.Request) {
	events := sse.NewWriter(w).Use(app.showcaseRenderers()...)
	_ = events.Comment("go-partial showcase stream")
	events.Flush()
	ctx := app.requestContext(r)

	for i := 1; i <= 5; i++ {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(700 * time.Millisecond):
		}

		status := partial.NewID("sse-status", "templates/sse_status.gohtml").
			SetFileSystem(os.DirFS("examples/showcase")).
			SetDot(SSEStatus{
				Step: i,
				Time: time.Now().Format("15:04:05"),
				Done: i == 5,
			})
		if err := events.PatchPartial(ctx, r, "#sse-status", status); err != nil {
			_ = events.Error(err)
			events.Flush()
			return
		}

		if err := events.Signal("progress", map[string]any{
			"step": i,
			"done": i == 5,
		}); err != nil {
			return
		}
		events.Flush()
	}
}
