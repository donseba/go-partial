package connector

import (
	"net/http"
)

type HTMX struct {
	base

	requestHeader               string
	boostedHeader               string
	historyRestoreRequestHeader string
}

func NewHTMX(c *Config) Connector {
	return &HTMX{
		base: base{
			config:       c,
			targetHeader: "HX-Target",
			selectHeader: "X-Select",
			actionHeader: "X-Action",
		},
		requestHeader:               "HX-Request",
		boostedHeader:               "HX-Boosted",
		historyRestoreRequestHeader: "HX-History-Restore-Request",
	}
}

func (h *HTMX) RenderPartial(r *http.Request) bool {
	hxRequest := r.Header.Get(h.requestHeader)
	hxBoosted := r.Header.Get(h.boostedHeader)
	hxHistoryRestoreRequest := r.Header.Get(h.historyRestoreRequestHeader)

	return (hxRequest == "true" || hxBoosted == "true") && hxHistoryRestoreRequest != "true"
}
