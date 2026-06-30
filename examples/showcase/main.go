package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/exp/actions"
	"github.com/donseba/go-partial/exp/csrf"
	"github.com/donseba/go-partial/exp/flash"
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
	extlogger "github.com/donseba/go-partial/ext/logger"
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
		logs:          newShowcaseLogs(120),
	}
	app.events = partial.NewAsyncEvents(
		partial.EventsConfig{Buffer: 256, DropPolicy: partial.DropNewest},
		extlogger.Sink(nil, extlogger.WithMinLevel(partial.EventWarn)),
		app.logs,
	)
	app.service = partial.NewService(&partial.Config{
		Connector:        connector.NewHTMX(nil),
		Events:           app.events,
		FS:               os.DirFS("examples/showcase"),
		Stages:           app.showcaseStages(),
		UseTemplateCache: false,
	})
	app.service.SetFunc(
		showcaseTranslationFunctions(),
		actions.FuncMap(),
		csrf.FuncMap(),
		extdebug.FuncMap(),
		flash.FuncMap(),
		extlogger.FuncMap(),
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
	mux.HandleFunc("/logger", app.loggerPage)
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

	addr := os.Getenv("SHOWCASE_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	displayAddr := addr
	if len(displayAddr) > 0 && displayAddr[0] == ':' {
		displayAddr = "localhost" + displayAddr
	}
	server := &http.Server{Addr: addr, Handler: mux}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("showcase shutdown error: %v", err)
		}
	}()

	log.Printf("showcase running on http://%s", displayAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := app.events.Close(closeCtx); err != nil {
		log.Printf("event shutdown error: %v", err)
	}
}

func (app *App) showcaseStages() []partial.RenderStage {
	return []partial.RenderStage{
		exterrors.Stage(exterrors.WithMode(exterrors.ModeDetailed)),
		extdebug.Stage(),
		extlogger.Stage(),
		actions.Stage(),
		csrf.Stage(),
		flash.Stage(
			flash.WithTemplate("templates/flash.gohtml"),
			flash.WithTargetTemplate("templates/flash_target.gohtml"),
		),
		interactions.Stage(showcaseInteractionRenderer()),
		localization.Stage(),
		metrics.Stage(metrics.Fanout(app.metrics, app.metricStreams), metrics.WithTag("chain", "showcase"), metrics.WithSlotName(slots.Name)),
		selection.Stage(),
		slots.Stage(),
		target.Stage(),
	}
}
