package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

type (
	App struct {
		PartialService *partial.Service
	}
)

func main() {
	logger := slog.Default()

	app := &App{
		PartialService: partial.NewService(&partial.Config{
			Logger: logger,
			Connector: connector.NewPartial(&connector.Config{
				UseURLQuery: true,
			}),
		}),
	}

	mux := http.NewServeMux()

	mux.Handle("GET /js/", http.StripPrefix("/js/", http.FileServer(http.Dir("../../js"))))

	mux.HandleFunc("GET /", app.home)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	logger.Info("starting server on :8080")
	err := server.ListenAndServe()
	if err != nil {
		logger.Error("error starting server", "error", err)
	}
}

// super simple example of how to use the partial service to render a layout with a content partial
func (a *App) home(w http.ResponseWriter, r *http.Request) {
	// layout, footer, index could be abstracted away and shared over multiple handlers within the same module, for instance.
	layout := a.PartialService.NewLayout()
	footer := partial.NewID("footer", filepath.Join("templates", "footer.gohtml"))
	index := partial.NewID("index", filepath.Join("templates", "index.gohtml")).WithOOB(footer)

	content := partial.NewID("content", filepath.Join("templates", "content.gohtml")).WithAction(func(ctx context.Context, p *partial.Partial, data *partial.Data) (*partial.Partial, error) {
		switch p.GetRequestedAction() {
		case "infinite-scroll":
			return handleInfiniteScroll(p, data)
		default:
			return p, nil
		}
	})

	// set the layout content and wrap it with the main template
	layout.Set(content).Wrap(index)

	err := layout.WriteWithRequest(r.Context(), w, r)
	if err != nil {
		http.Error(w, fmt.Errorf("error rendering layout: %w", err).Error(), http.StatusInternalServerError)
	}
}

type (
	Row struct {
		ID   int
		Name string
		Desc string
	}
)

func handleInfiniteScroll(p *partial.Partial, data *partial.Data) (*partial.Partial, error) {
	startID := 0
	if p.GetRequest().URL.Query().Get("ID") != "" {
		startID, _ = strconv.Atoi(p.GetRequest().URL.Query().Get("ID"))
	}

	if startID >= 100 {
		p.SetResponseHeaders(map[string]string{
			"X-Swap":            "innerHTML",
			"X-Infinite-Scroll": "stop",
		})
		p = partial.NewID("rickrolled", filepath.Join("templates", "rickrolled.gohtml"))
	} else {
		data.Data["Rows"] = generateNextRows(startID, 10)
	}

	return p, nil
}

func generateNextRows(lastID int, count int) []Row {
	var out []Row
	start := lastID + 1
	end := lastID + count

	for i := start; i <= end; i++ {
		out = append(out, Row{
			ID:   i,
			Name: fmt.Sprintf("Name %d", i),
			Desc: fmt.Sprintf("Description %d", i),
		})
	}

	return out
}
