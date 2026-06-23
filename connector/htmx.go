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

const (
	HTMXHeaderBoosted               HeaderKey = "HX-Boosted"
	HTMXHeaderCurrentURL            HeaderKey = "HX-Current-URL"
	HTMXHeaderHistoryRestoreRequest HeaderKey = "HX-History-Restore-Request"
	HTMXHeaderPrompt                HeaderKey = "HX-Prompt"
	HTMXHeaderRequest               HeaderKey = "HX-Request"
	HTMXHeaderTarget                HeaderKey = "HX-Target"
	HTMXHeaderTriggerName           HeaderKey = "HX-Trigger-Name"
	HTMXHeaderTriggerRequest        HeaderKey = "HX-Trigger"

	HTMXHeaderLocation           HeaderKey = "HX-Location"
	HTMXHeaderPushURL            HeaderKey = "HX-Push-Url"
	HTMXHeaderRedirect           HeaderKey = "HX-Redirect"
	HTMXHeaderRefresh            HeaderKey = "HX-Refresh"
	HTMXHeaderReplaceURL         HeaderKey = "HX-Replace-Url"
	HTMXHeaderReswap             HeaderKey = "HX-Reswap"
	HTMXHeaderRetarget           HeaderKey = "HX-Retarget"
	HTMXHeaderReselect           HeaderKey = "HX-Reselect"
	HTMXHeaderTrigger            HeaderKey = "HX-Trigger"
	HTMXHeaderTriggerAfterSettle HeaderKey = "HX-Trigger-After-Settle"
	HTMXHeaderTriggerAfterSwap   HeaderKey = "HX-Trigger-After-Swap"
)

func NewHTMX(c *Config) Connector {
	return &HTMX{
		base: base{
			config:       c,
			targetHeader: HTMXHeaderTarget.String(),
			selectHeader: HeaderSelect.String(),
			actionHeader: HeaderAction.String(),
		},
		requestHeader:               HTMXHeaderRequest.String(),
		boostedHeader:               HTMXHeaderBoosted.String(),
		historyRestoreRequestHeader: HTMXHeaderHistoryRestoreRequest.String(),
	}
}

func (h *HTMX) RenderPartial(r *http.Request) bool {
	hxRequest := r.Header.Get(h.requestHeader)
	hxBoosted := r.Header.Get(h.boostedHeader)
	hxHistoryRestoreRequest := r.Header.Get(h.historyRestoreRequestHeader)

	return (hxRequest == "true" || hxBoosted == "true") && hxHistoryRestoreRequest != "true"
}

func (h *HTMX) ResponseHeaders(response Response) map[string]string {
	headers := make(map[string]string)
	setResponseHeader(headers, HTMXHeaderLocation, response.Location)
	setResponseHeader(headers, HTMXHeaderPushURL, response.PushURL)
	setResponseHeader(headers, HTMXHeaderRedirect, response.Redirect)
	if response.Refresh != nil {
		setResponseHeader(headers, HTMXHeaderRefresh, boolString(*response.Refresh))
	}
	setResponseHeader(headers, HTMXHeaderReplaceURL, response.ReplaceURL)
	setResponseHeader(headers, HTMXHeaderReswap, response.Reswap)
	setResponseHeader(headers, HTMXHeaderRetarget, response.Retarget)
	setResponseHeader(headers, HTMXHeaderReselect, response.Reselect)
	setResponseHeader(headers, HTMXHeaderTrigger, response.Trigger)
	setResponseHeader(headers, HTMXHeaderTriggerAfterSettle, response.TriggerAfterSettle)
	setResponseHeader(headers, HTMXHeaderTriggerAfterSwap, response.TriggerAfterSwap)
	return headers
}
