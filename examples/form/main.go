package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/donseba/go-partial"
	"log/slog"
	"net/http"
	"path/filepath"
)

type (
	App struct {
		PartialService *partial.Service
	}

	FormData struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		HiddenField string `json:"hiddenField"`
	}
)

func main() {
	logger := slog.Default()

	app := &App{
		PartialService: partial.NewService(&partial.Config{
			Logger: logger,
		}),
	}

	mux := http.NewServeMux()

	mux.Handle("GET /js/", http.StripPrefix("/js/", http.FileServer(http.Dir("../../js"))))
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("POST /", app.home)

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

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	layout := a.PartialService.NewLayout()
	footer := partial.NewID("footer", filepath.Join("templates", "footer.gohtml"))
	index := partial.NewID("index", filepath.Join("templates", "index.gohtml")).WithOOB(footer)
	content := partial.NewID("form", filepath.Join("templates", "form.gohtml")).WithAction(func(ctx context.Context, p *partial.Partial, data *partial.Data) (*partial.Partial, error) {
		switch p.GetRequestedAction() {
		case "submit":
			formData := &FormData{}
			err := json.NewDecoder(r.Body).Decode(formData)
			if err != nil {
				return nil, fmt.Errorf("error decoding form data: %w", err)
			}

			w.Header().Set("X-Event-Notify", `{"type": "success", "message": "Form submitted successfully"}`)
			p = p.Templates(filepath.Join("templates", "submitted.gohtml")).AddData("formData", formData)
		}

		return p, nil
	})

	layout.Set(content).Wrap(index)

	err := layout.WriteWithRequest(r.Context(), w, r)
	if err != nil {
		http.Error(w, fmt.Errorf("error rendering layout: %w", err).Error(), http.StatusInternalServerError)
	}
}
