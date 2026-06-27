package main

import (
	"log"
	"net/http"
	"os"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func main() {
	app := &App{
		service: partial.NewService(&partial.Config{
			Connector:        connector.NewHTMX(nil),
			FS:               os.DirFS("examples/showcase"),
			ErrorMode:        partial.ErrorModeDetailed,
			UseTemplateCache: false,
		}),
		rows: []Row{
			{ID: 1, Name: "Coffee", Price: "12.50", Status: "Ready", Owner: "Ada"},
			{ID: 2, Name: "Tea", Price: "4.25", Status: "Brewing", Owner: "Ada"},
			{ID: 3, Name: "Cake", Price: "6.75", Status: "Queued", Owner: "Ada"},
		},
		products:     fakeProducts(),
		carts:        make(map[string]map[int]int),
		flowSessions: make(map[string]*partial.FlowSessionData),
	}
	app.service.UseFuncs(showcaseTranslationFunctions())
	app.service.SetInteractionRenderer(showcaseInteractionRenderer())

	app.service.SetData(map[string]any{
		"AppName": "go-partial showcase",
		"Now":     time.Now().Format(time.RFC822),
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	mux.HandleFunc("/scoped", app.scoped)
	mux.HandleFunc("/scoped/refresh-row", app.refreshRow)
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
	mux.HandleFunc("/infinite", app.infinite)
	mux.HandleFunc("/infinite/load", app.infiniteLoad)
	mux.HandleFunc("/shop", app.shop)
	mux.HandleFunc("/shop/load", app.shopLoad)
	mux.HandleFunc("/shop/cart/add", app.shopCartAdd)
	mux.HandleFunc("/shop/cart/remove", app.shopCartRemove)
	mux.HandleFunc("/shop/cart/open", app.shopCartOpen)
	mux.HandleFunc("/error", app.errorPage)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("examples/showcase/static"))))

	log.Println("showcase running on http://localhost:8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}
