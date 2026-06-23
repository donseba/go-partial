package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
)

func (app *App) interactions(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/interactions.gohtml").
		SetData(map[string]any{
			"Title": "Interaction helpers",
		}).
		SetInteractions(map[string]any{
			"Async":         partial.Async("/interactions/async"),
			"Poll":          partial.Poll("/interactions/poll").Every(3 * time.Second),
			"On":            partial.On("showcase:ping", "/interactions/on").ID("on-listener").Target("#on-target").Placeholder(""),
			"Refresh":       partial.Refresh("/interactions/refresh").ID("refresh-trigger").Target("#refresh-panel").Placeholder("Refresh panel"),
			"Island":        partial.Island("profile", "/interactions/island"),
			"IslandRefresh": partial.Refresh("/interactions/island").ID("island-refresh").Target("#island-profile").Placeholder("Refresh island"),
			"Stream":        partial.Stream("/interactions/stream").Placeholder("Waiting for stream..."),
			"Prefetch":      partial.Prefetch("/interactions/async"),
			"Reveal":        partial.Reveal("/interactions/reveal"),
		})
	app.renderPartial(w, r, content)
}

func (app *App) interactionsAsync(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-async", "templates/interaction_result_inner.gohtml").SetData(map[string]any{
		"ID":      "async-interactions-async",
		"Label":   "Async",
		"Message": "Loaded after the page shell rendered.",
		"Time":    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsReveal(w http.ResponseWriter, r *http.Request) {
	time.Sleep(2 * time.Second)
	content := partial.NewID("interaction-reveal", "templates/interaction_result_inner.gohtml").SetData(map[string]any{
		"ID":      "reveal-interactions-reveal",
		"Label":   "Reveal",
		"Message": "Loaded when the placeholder entered the viewport.",
		"Time":    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsPoll(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-poll", "templates/interaction_result_inner.gohtml").SetData(map[string]any{
		"ID":      "poll-interactions-poll",
		"Label":   "Poll",
		"Message": "Refreshed by a polling trigger.",
		"Time":    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsOn(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-on", "templates/interaction_result_inner.gohtml").SetData(map[string]any{
		"ID":      "on-target",
		"Label":   "Event",
		"Message": "Updated after a custom browser event.",
		"Time":    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsIsland(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-island", "templates/interaction_result_inner.gohtml").SetData(map[string]any{
		"ID":      "island-profile",
		"Label":   "Island",
		"Message": "A named lazy island rendered by the server.",
		"Time":    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsRefresh(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-refresh", "templates/interaction_result_inner.gohtml").SetData(map[string]any{
		"ID":      "refresh-interactions-refresh",
		"Label":   "Refresh",
		"Message": "Rendered by an explicit refresh interaction.",
		"Time":    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	select {
	case <-r.Context().Done():
		return
	case <-time.After(1200 * time.Millisecond):
	}

	content := partial.NewID("interaction-stream", "templates/interaction_result_inner.gohtml").
		SetFileSystem(os.DirFS("examples/showcase")).
		SetData(map[string]any{
			"ID":      "stream-interactions-stream",
			"Label":   "Stream",
			"Message": "Received over an SSE message.",
			"Time":    time.Now().Format("15:04:05"),
		})
	out, err := content.Render(app.requestContext(r))
	if err != nil {
		if _, writeErr := fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error()); writeErr != nil {
			return
		}
		flusher.Flush()
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return
		}
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return
	}
	flusher.Flush()
}
