package connector

import (
	"net/http"
	"strings"
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

	HTMXAttrGet        = "hx-get"
	HTMXAttrExt        = "hx-ext"
	HTMXAttrTrigger    = "hx-trigger"
	HTMXAttrTarget     = "hx-target"
	HTMXAttrSwap       = "hx-swap"
	HTMXAttrSSEConnect = "sse-connect"
	HTMXAttrSSESwap    = "sse-swap"
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
	if r == nil {
		return false
	}
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

func (h *HTMX) InteractionAttrs(interaction Interaction) map[string]string {
	attrs := map[string]string{}
	target := "#" + interaction.ID
	if interaction.Target != "" {
		target = interaction.Target
	}
	swap := interaction.Swap
	if swap == "" {
		swap = string(SwapInnerHTML)
	}

	switch interaction.Kind {
	case InteractionReveal:
		attrs[HTMXAttrGet] = interaction.URL
		attrs[HTMXAttrTrigger] = "revealed"
		attrs[HTMXAttrTarget] = target
		attrs[HTMXAttrSwap] = swap
	case InteractionPoll:
		interval := interaction.Interval
		if interval == "" {
			interval = "5s"
		}
		if interaction.Swap == "" {
			swap = "innerHTML"
		}
		attrs[HTMXAttrGet] = interaction.URL
		attrs[HTMXAttrTrigger] = "every " + interval
		attrs[HTMXAttrTarget] = target
		attrs[HTMXAttrSwap] = swap
	case InteractionStream:
		attrs[HTMXAttrExt] = "sse"
		attrs[HTMXAttrSSEConnect] = interaction.URL
		attrs[HTMXAttrSSESwap] = "message"
	case InteractionPrefetch:
		attrs["rel"] = "prefetch"
		attrs["href"] = interaction.URL
	case InteractionRefresh:
		trigger := interaction.Trigger
		if trigger == "" {
			trigger = "click"
		}
		attrs[HTMXAttrGet] = interaction.URL
		attrs[HTMXAttrTrigger] = trigger
		attrs[HTMXAttrTarget] = target
		attrs[HTMXAttrSwap] = swap
	case InteractionOn:
		trigger := interaction.Trigger
		if trigger == "" {
			trigger = interaction.Name
		}
		if from := interaction.Options["from"]; from != "" && !strings.Contains(trigger, " from:") {
			trigger += " from:" + from
		}
		attrs[HTMXAttrGet] = interaction.URL
		attrs[HTMXAttrTrigger] = trigger
		attrs[HTMXAttrTarget] = target
		attrs[HTMXAttrSwap] = swap
	default:
		attrs[HTMXAttrGet] = interaction.URL
		attrs[HTMXAttrTrigger] = "load"
		attrs[HTMXAttrTarget] = target
		attrs[HTMXAttrSwap] = swap
	}

	return attrs
}
