package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/interactions"
)

func (app *App) interactions(w http.ResponseWriter, r *http.Request) {
	asyncPartial := partial.NewID("async-interactions", "templates/interaction_result_inner.gohtml")

	content := partial.NewID("content", "templates/interactions.gohtml").
		SetDot(InteractionPage{
			Title: "Interaction helpers",
		}).
		SetContract("interaction",
			interactions.NewAsync("/interactions/async"),
			interactions.NewPoll("/interactions/poll").Every(3*time.Second),
			interactions.NewOn("showcase:ping", "/interactions/on").ID("on-listener").Target("#on-target").Placeholder(""),
			interactions.NewRefresh("/interactions/refresh").ID("refresh-trigger").Target("#refresh-panel").Placeholder("Refresh panel"),
			interactions.NewAsync("/interactions/profile").ID("profile"),
			interactions.NewRefresh("/interactions/profile").As("ProfileRefresh").ID("profile-refresh").Target("#profile").Placeholder("Refresh profile"),
			interactions.NewStream("/interactions/stream").Placeholder("Waiting for stream..."),
			interactions.NewPrefetch("/interactions/async").As("Prefetch"),
			interactions.NewReveal("/interactions/reveal"),
		).
		With(asyncPartial)

	app.renderPartial(w, r, content)
}

func (app *App) interactionsAsync(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-async", "templates/interaction_result_inner.gohtml").SetDot(InteractionResult{
		ID:      "async-interactions-async",
		Label:   "Async",
		Message: "Loaded after the page shell rendered.",
		Time:    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsReveal(w http.ResponseWriter, r *http.Request) {
	time.Sleep(2 * time.Second)
	content := partial.NewID("interaction-reveal", "templates/interaction_result_inner.gohtml").SetDot(InteractionResult{
		ID:      "reveal-interactions-reveal",
		Label:   "Reveal",
		Message: "Loaded when the placeholder entered the viewport.",
		Time:    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsPoll(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-poll", "templates/interaction_result_inner.gohtml").SetDot(InteractionResult{
		ID:      "poll-interactions-poll",
		Label:   "Poll",
		Message: "Refreshed by a polling trigger.",
		Time:    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsOn(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-on", "templates/interaction_result_inner.gohtml").SetDot(InteractionResult{
		ID:      "on-target",
		Label:   "Event",
		Message: "Updated after a custom browser event.",
		Time:    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsProfile(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-profile", "templates/interaction_result_inner.gohtml").SetDot(InteractionResult{
		ID:      "profile",
		Label:   "Async",
		Message: "A named async region rendered by the server.",
		Time:    time.Now().Format("15:04:05"),
	})
	app.writeStandalone(w, r, content)
}

func (app *App) interactionsRefresh(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("interaction-refresh", "templates/interaction_result_inner.gohtml").SetDot(InteractionResult{
		ID:      "refresh-interactions-refresh",
		Label:   "Refresh",
		Message: "Rendered by an explicit refresh interaction.",
		Time:    time.Now().Format("15:04:05"),
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
		SetDot(InteractionResult{
			ID:      "stream-interactions-stream",
			Label:   "Stream",
			Message: "Received over an SSE message.",
			Time:    time.Now().Format("15:04:05"),
		})
	content = app.configureStandalone(content, nil)
	out, err := partial.Render(app.requestContext(r), content)
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
