package main

import (
	"net/http"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func (app *App) headers(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/headers.gohtml", PageTitle{
		Title: "HTMX response helpers",
	})
}

func (app *App) headersNotify(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("notice", "templates/notice.gohtml").SetDot(NoticePage{
		Message: "Response headers changed this element at " + time.Now().Format("15:04:05") + ".",
	})
	content.Response().
		Retarget("#notice").
		ReswapWith(connector.NewSwap().Style(connector.SwapOuterHTML).Swap(120 * time.Millisecond).Transition(true)).
		TriggerWith(connector.NewTrigger().AddEventObject("showcase:notice", map[string]any{
			"message": "Headers set at " + time.Now().Format("15:04:05"),
		}))
	app.writeContent(w, r, content)
}
