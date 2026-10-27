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
	// HTMXHeaderPrompt is sent by HTMX 2 core; HTMX 4 requires hx-prompt.
	HTMXHeaderPrompt  HeaderKey = "HX-Prompt"
	HTMXHeaderRequest HeaderKey = "HX-Request"
	HTMXHeaderTarget  HeaderKey = "HX-Target"
	// HTMX 2 request keys. HTMX 4 replaces both with HTMXHeaderSource.
	HTMXHeaderTriggerName    HeaderKey = "HX-Trigger-Name"
	HTMXHeaderTriggerRequest HeaderKey = "HX-Trigger"

	HTMXHeaderLocation   HeaderKey = "HX-Location"
	HTMXHeaderPushURL    HeaderKey = "HX-Push-Url"
	HTMXHeaderRedirect   HeaderKey = "HX-Redirect"
	HTMXHeaderRefresh    HeaderKey = "HX-Refresh"
	HTMXHeaderReplaceURL HeaderKey = "HX-Replace-Url"
	HTMXHeaderReswap     HeaderKey = "HX-Reswap"
	HTMXHeaderRetarget   HeaderKey = "HX-Retarget"
	HTMXHeaderReselect   HeaderKey = "HX-Reselect"
	HTMXHeaderTrigger    HeaderKey = "HX-Trigger"
	// Timing response headers are HTMX 2-only; NewHTMX4 omits them.
	HTMXHeaderTriggerAfterSettle HeaderKey = "HX-Trigger-After-Settle"
	HTMXHeaderTriggerAfterSwap   HeaderKey = "HX-Trigger-After-Swap"

	HTMXAttrGet = "hx-get"
	// HTMXAttrExt is HTMX 2-only. HTMX 4 loads extension scripts directly.
	HTMXAttrExt     = "hx-ext"
	HTMXAttrTrigger = "hx-trigger"
	HTMXAttrTarget  = "hx-target"
	HTMXAttrSwap    = "hx-swap"
	HTMXAttrSelect  = "hx-select"
	// HTMX 2 SSE extension attributes. HTMX 4 uses HTMX4AttrSSEConnect.
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
	hxRequest := headerValue(r.Header, h.requestHeader)
	hxBoosted := headerValue(r.Header, h.boostedHeader)
	hxHistoryRestoreRequest := headerValue(r.Header, h.historyRestoreRequestHeader)

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
	case InteractionSubmit:
		addHTMXSubmit(attrs, interaction, target, swap)
	case InteractionLive:
		addHTMXLive(attrs, interaction, target, swap)
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

	addHTMXRequestOptions(attrs, interaction)

	return withInteractionAttrs(attrs, interaction)
}

// addHTMXSubmit adds the htmx attributes of a submit interaction: a request
// with its method, hx-post by default, swapped into target.
func addHTMXSubmit(attrs map[string]string, interaction Interaction, target, swap string) {
	attrs[htmxMethodAttr(interaction.Options[OptionMethod])] = interaction.URL
	attrs[HTMXAttrTarget] = target
	attrs[HTMXAttrSwap] = swap
	setAttr(attrs, HTMXAttrTrigger, interaction.Trigger)
}

// addHTMXLive adds the htmx attributes of a live interaction: a request as
// the user types, after a pause, swapped into target.
func addHTMXLive(attrs map[string]string, interaction Interaction, target, swap string) {
	trigger := interaction.Trigger
	if trigger == "" {
		trigger = "input changed delay:" + liveDelay(interaction) + ", search"
	}

	attrs[HTMXAttrGet] = interaction.URL
	attrs[HTMXAttrTrigger] = trigger
	attrs[HTMXAttrTarget] = target
	attrs[HTMXAttrSwap] = swap
}

// addHTMXRequestOptions adds the options of an interaction that requests,
// such as its hx-select and hx-include, to its htmx attributes.
func addHTMXRequestOptions(attrs map[string]string, interaction Interaction) {
	if !requests(attrs) {
		return
	}

	setAttr(attrs, HTMXAttrSelect, interaction.Select)
	setAttr(attrs, "hx-include", interaction.Options[OptionInclude])
	setAttr(attrs, "hx-push-url", interaction.Options[OptionPushURL])
	setAttr(attrs, "hx-replace-url", interaction.Options[OptionReplaceURL])
}

// htmxMethods are the request attributes of htmx, by method.
var htmxMethods = map[string]string{
	"get":    HTMXAttrGet,
	"post":   "hx-post",
	"put":    "hx-put",
	"patch":  "hx-patch",
	"delete": "hx-delete",
}

// htmxMethodAttr is the htmx attribute that requests with method; hx-post
// for an empty or unknown one.
func htmxMethodAttr(method string) string {
	if attr, ok := htmxMethods[strings.ToLower(strings.TrimSpace(method))]; ok {
		return attr
	}

	return "hx-post"
}

// requests reports whether the htmx attributes make a request.
func requests(attrs map[string]string) bool {
	for _, attr := range htmxMethods {
		if attrs[attr] != "" {
			return true
		}
	}

	return false
}

// liveDelay is the pause in typing a live interaction waits for.
func liveDelay(interaction Interaction) string {
	if delay := strings.TrimSpace(interaction.Options[OptionDelay]); delay != "" {
		return delay
	}

	return "300ms"
}
