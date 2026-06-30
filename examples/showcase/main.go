package main

import (
	"log"
	"net/http"
	"os"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/exp/actions"
	"github.com/donseba/go-partial/exp/csrf"
	"github.com/donseba/go-partial/exp/interactions"
	"github.com/donseba/go-partial/exp/localization"
	"github.com/donseba/go-partial/exp/metrics"
	"github.com/donseba/go-partial/exp/pageflow"
	"github.com/donseba/go-partial/exp/selection"
	"github.com/donseba/go-partial/exp/slots"
	"github.com/donseba/go-partial/exp/target"
	"github.com/donseba/go-partial/exp/templatehelpers"
	extdebug "github.com/donseba/go-partial/ext/debug"
	exterrors "github.com/donseba/go-partial/ext/errors"
)

func main() {
	app := &App{
		rows: []Row{
			{ID: 1, Name: "Coffee", Price: "12.50", Status: "Ready", Owner: "Ada"},
			{ID: 2, Name: "Tea", Price: "4.25", Status: "Brewing", Owner: "Ada"},
			{ID: 3, Name: "Cake", Price: "6.75", Status: "Queued", Owner: "Ada"},
		},
		products:      fakeProducts(),
		carts:         make(map[string]map[int]int),
		flowSessions:  make(map[string]*pageflow.SessionData),
		metrics:       newShowcaseMetrics(80),
		metricStreams: newMetricStreamHub(),
	}
	app.service = partial.NewService(&partial.Config{
		Connector:        connector.NewHTMX(nil),
		FS:               os.DirFS("examples/showcase"),
		Renderers:        app.showcaseRenderers(),
		UseTemplateCache: false,
	})
	app.service.SetFunc(
		showcaseTranslationFunctions(),
		actions.FuncMap(),
		csrf.FuncMap(),
		extdebug.FuncMap(),
		interactions.FuncMap(),
		localization.FuncMap(),
		selection.FuncMap(),
		slots.FuncMap(),
		target.FuncMap(),
		templatehelpers.FuncMap(),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	mux.HandleFunc("/rows", app.rowsPage)
	mux.HandleFunc("/rows/refresh-row", app.refreshRow)
	mux.HandleFunc("/selection", app.selection)
	mux.HandleFunc("/tabs", app.tabs)
	mux.HandleFunc("/action", app.action)
	mux.HandleFunc("/async", app.asyncPage)
	mux.HandleFunc("/async/stats", app.asyncStats)
	mux.HandleFunc("/async/row/", app.asyncRow)
	mux.HandleFunc("/interactions", app.interactions)
	mux.HandleFunc("/interactions/async", app.interactionsAsync)
	mux.HandleFunc("/interactions/reveal", app.interactionsReveal)
	mux.HandleFunc("/interactions/poll", app.interactionsPoll)
	mux.HandleFunc("/interactions/on", app.interactionsOn)
	mux.HandleFunc("/interactions/profile", app.interactionsProfile)
	mux.HandleFunc("/interactions/refresh", app.interactionsRefresh)
	mux.HandleFunc("/interactions/stream", app.interactionsStream)
	mux.HandleFunc("/oob", app.oob)
	mux.HandleFunc("/oob/ping", app.oobPing)
	mux.HandleFunc("/context", app.contextPage)
	mux.HandleFunc("/debug", app.debugPage)
	mux.HandleFunc("/localization", app.localization)
	mux.HandleFunc("/headers", app.headers)
	mux.HandleFunc("/headers/notify", app.headersNotify)
	mux.HandleFunc("/flow", app.flow)
	mux.HandleFunc("/sse", app.sse)
	mux.HandleFunc("/sse/stream", app.sseStream)
	mux.HandleFunc("/metrics", app.metricsPage)
	mux.HandleFunc("/metrics/live", app.liveMetricsPage)
	mux.HandleFunc("/metrics/live/stream", app.liveMetricsStream)
	mux.HandleFunc("/metrics/live/ping", app.liveMetricsPing)
	mux.HandleFunc("/infinite", app.infinite)
	mux.HandleFunc("/infinite/load", app.infiniteLoad)
	mux.HandleFunc("/shop", app.shop)
	mux.HandleFunc("/shop/load", app.shopLoad)
	mux.HandleFunc("/shop/cart/add", app.shopCartAdd)
	mux.HandleFunc("/shop/cart/remove", app.shopCartRemove)
	mux.HandleFunc("/shop/cart/open", app.shopCartOpen)
	mux.HandleFunc("/error", app.errorPage)
	mux.HandleFunc("/error/section", app.errorSection)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("examples/showcase/static"))))

	log.Println("showcase running on http://localhost:8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}

func (app *App) showcaseRenderers() []partial.Renderer {
	return []partial.Renderer{
		exterrors.Renderer(exterrors.WithMode(exterrors.ModeDetailed)),
		extdebug.Renderer(),
		actions.Renderer(),
		csrf.Renderer(),
		interactions.Renderer(showcaseInteractionRenderer()),
		localization.Renderer(),
		metrics.Renderer(metrics.Fanout(app.metrics, app.metricStreams), metrics.WithTag("chain", "showcase"), metrics.WithSlotName(slots.Name)),
		selection.Renderer(),
		slots.Renderer(),
		target.Renderer(),
	}
}
