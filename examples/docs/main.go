package main

import (
	"context"
	"log"
	"net/http"
	"os"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

type NavItem struct {
	Path  string
	Label string
	Group string
}

type App struct {
	service *partial.Service
}

func main() {
	app := &App{
		service: partial.NewService(&partial.Config{
			Connector: connector.NewHTMX(nil),
			FS:        os.DirFS("examples/docs"),
			ErrorMode: partial.ErrorModeDetailed,
		}),
	}
	app.service.SetData(map[string]any{
		"AppName": "go-partial",
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.overview)
	mux.HandleFunc("/docs/installation", app.installation)
	mux.HandleFunc("/docs/rendering", app.rendering)
	mux.HandleFunc("/docs/data-context", app.dataContext)
	mux.HandleFunc("/docs/selection-action", app.selectionAction)
	mux.HandleFunc("/docs/flow", app.flow)
	mux.HandleFunc("/docs/localization", app.localization)
	mux.HandleFunc("/docs/integrations", app.integrations)
	mux.HandleFunc("/docs/integrations/htmx", app.htmx)
	mux.HandleFunc("/docs/integrations/sse", app.sse)
	mux.HandleFunc("/docs/integrations/custom-clients", app.customClients)
	mux.HandleFunc("/docs/api", app.api)
	mux.HandleFunc("/docs/api/pageflow", app.pageFlowAPI)
	mux.HandleFunc("/docs/target-resolver", app.targetResolver)
	mux.HandleFunc("/docs/connectors", app.connectors)
	mux.HandleFunc("/docs/template-functions", app.templateFunctions)
	mux.HandleFunc("/docs/htmx", app.htmx)
	mux.HandleFunc("/docs/error-boundaries", app.errorBoundaries)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("examples/docs/static"))))

	log.Println("docs running on http://localhost:8091")
	log.Fatal(http.ListenAndServe(":8091", mux))
}

func (app *App) overview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	app.render(w, r, "templates/docs_overview.gohtml")
}

func (app *App) installation(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_installation.gohtml")
}

func (app *App) rendering(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_rendering.gohtml")
}

func (app *App) dataContext(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_data_context.gohtml")
}

func (app *App) selectionAction(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_selection_action.gohtml")
}

func (app *App) flow(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_flow.gohtml")
}

func (app *App) localization(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_localization.gohtml")
}

func (app *App) integrations(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_integrations.gohtml")
}

func (app *App) sse(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_sse.gohtml")
}

func (app *App) customClients(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_custom_clients.gohtml")
}

func (app *App) api(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_api.gohtml")
}

func (app *App) pageFlowAPI(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_pageflow_api.gohtml")
}

func (app *App) targetResolver(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_target_resolver.gohtml")
}

func (app *App) connectors(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_connectors.gohtml")
}

func (app *App) templateFunctions(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_template_functions.gohtml")
}

func (app *App) htmx(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_htmx.gohtml")
}

func (app *App) errorBoundaries(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "templates/docs_error_boundaries.gohtml")
}

func (app *App) render(w http.ResponseWriter, r *http.Request, tmpl string) {
	content := partial.NewID("content", tmpl)
	wrapper := partial.NewID("layout", "templates/layout.gohtml")
	wrapper.WithOOB(partial.NewID("header", "templates/header.gohtml").SetData(map[string]any{
		"Nav": app.navItems(),
	}).SetAlwaysSwapOOB(true))
	wrapper.WithOOB(partial.NewID("sidebar", "templates/sidebar.gohtml").SetData(map[string]any{
		"Nav": app.navItems(),
	}).SetAlwaysSwapOOB(true))

	layout := app.service.NewLayout().Set(content).Wrap(wrapper)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := layout.WriteWithRequest(context.Background(), w, r); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (app *App) navItems() []NavItem {
	return []NavItem{
		{Path: "/", Label: "Overview", Group: "Guide"},
		{Path: "/docs/installation", Label: "Installation", Group: "Guide"},
		{Path: "/docs/rendering", Label: "Rendering model", Group: "Guide"},
		{Path: "/docs/data-context", Label: "Data and context", Group: "Guide"},
		{Path: "/docs/selection-action", Label: "Selection and action", Group: "Guide"},
		{Path: "/docs/flow", Label: "Page flows", Group: "Guide"},
		{Path: "/docs/target-resolver", Label: "Target resolver", Group: "Guide"},
		{Path: "/docs/localization", Label: "Localization", Group: "Guide"},
		{Path: "/docs/error-boundaries", Label: "Error boundaries", Group: "Guide"},
		{Path: "/docs/integrations", Label: "Overview", Group: "Integration"},
		{Path: "/docs/integrations/htmx", Label: "HTMX", Group: "Integration"},
		{Path: "/docs/integrations/sse", Label: "Server-sent events", Group: "Integration"},
		{Path: "/docs/integrations/custom-clients", Label: "Custom clients", Group: "Integration"},
		{Path: "/docs/api", Label: "Core API", Group: "Reference"},
		{Path: "/docs/api/pageflow", Label: "PageFlow API", Group: "Reference"},
		{Path: "/docs/template-functions", Label: "Template functions", Group: "Reference"},
		{Path: "/docs/connectors", Label: "Connectors", Group: "Reference"},
	}
}
