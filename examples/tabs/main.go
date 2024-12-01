package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"

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
	// the tabs for this page.
	selectMap := map[string]*partial.Partial{
		"tab1": partial.New(filepath.Join("templates", "tab1.gohtml")),
		"tab2": partial.New(filepath.Join("templates", "tab2.gohtml")),
		"tab3": partial.New(filepath.Join("templates", "tab3.gohtml")),
	}

	// layout, footer, index could be abstracted away and shared over multiple handlers within the same module, for instance.
	layout := a.PartialService.NewLayout()
	footer := partial.NewID("footer", filepath.Join("templates", "footer.gohtml"))
	index := partial.NewID("index", filepath.Join("templates", "index.gohtml")).WithOOB(footer)

	content := partial.NewID("content", filepath.Join("templates", "content.gohtml")).WithSelectMap("tab1", selectMap)

	// set the layout content and wrap it with the main template
	layout.Set(content).Wrap(index)

	err := layout.WriteWithRequest(r.Context(), w, r)
	if err != nil {
		http.Error(w, fmt.Errorf("error rendering layout: %w", err).Error(), http.StatusInternalServerError)
	}
}
