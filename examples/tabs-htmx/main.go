package main

import (
	"fmt"
	"github.com/donseba/go-partial"
	"log/slog"
	"net/http"
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
			PartialHeader: "HX-Target",
			Logger:        logger,
		}),
	}

	mux := http.NewServeMux()

	mux.Handle("GET /files/", http.StripPrefix("/files/", http.FileServer(http.Dir("./files"))))

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
		"tab1": partial.New("tab1.gohtml"),
		"tab2": partial.New("tab2.gohtml"),
		"tab3": partial.New("tab3.gohtml"),
	}

	// layout, footer, index could be abstracted away and shared over multiple handlers within the same module, for instance.
	layout := a.PartialService.NewLayout()
	footer := partial.NewID("footer", "footer.gohtml")
	index := partial.NewID("index", "index.gohtml").WithOOB(footer)

	content := partial.NewID("content", "content.gohtml").WithSelectMap("tab1", selectMap)

	// set the layout content and wrap it with the main template
	layout.Set(content).Wrap(index)

	err := layout.WriteWithRequest(r.Context(), w, r)
	if err != nil {
		http.Error(w, fmt.Errorf("error rendering layout: %w", err).Error(), http.StatusInternalServerError)
	}
}
