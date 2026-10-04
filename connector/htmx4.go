package connector

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// HTMX4 implements the htmx 4 protocol. NewHTMX remains the htmx 2 connector.
type HTMX4 struct{ *HTMX }

const (
	HTMXHeaderRequestType HeaderKey = "HX-Request-Type"
	HTMXHeaderSource      HeaderKey = "HX-Source"
	HTMX4AttrSSEConnect             = "hx-sse:connect"
)

func NewHTMX4(c *Config) Connector {
	return &HTMX4{HTMX: NewHTMX(c).(*HTMX)}
}

func (h *HTMX4) RenderPartial(r *http.Request) bool {
	if !h.HTMX.RenderPartial(r) {
		return false
	}

	// Body swaps, hx-select and history restoration need the whole document.
	switch r.Header.Get(HTMXHeaderRequestType.String()) {
	case "", "partial":
		return true
	default:
		return false
	}
}

func (h *HTMX4) GetTargetValue(r *http.Request) string {
	target := h.HTMX.GetTargetValue(r)
	if _, id, ok := strings.Cut(target, "#"); ok {
		// htmx 4 sends tagName#encodeURI(id), rather than the bare ID.
		if decoded, err := url.PathUnescape(id); err == nil {
			return decoded
		}

		return id
	}

	return target
}

// ResponseHeaders omits the two timing headers removed by htmx 4. Use Trigger
// for server events, or browser after:swap / after:settle listeners for timing.
func (h *HTMX4) ResponseHeaders(response Response) map[string]string {
	if response.swap != nil && response.Reswap == response.swap.String() {
		response.Reswap = response.swap.format(true)
	}

	headers := h.HTMX.ResponseHeaders(response)
	delete(headers, HTMXHeaderTriggerAfterSwap.String())
	delete(headers, HTMXHeaderTriggerAfterSettle.String())

	return headers
}

func (h *HTMX4) InteractionAttrs(interaction Interaction) map[string]string {
	if interaction.Kind == InteractionOn {
		if from := interaction.Options["from"]; strings.ContainsAny(from, " ,\t\n\"'") {
			options := make(map[string]string, len(interaction.Options))
			for key, value := range interaction.Options {
				options[key] = value
			}

			options["from"] = strconv.Quote(from)
			interaction.Options = options
		}
	}

	attrs := h.HTMX.InteractionAttrs(interaction)
	if interaction.Kind == InteractionStream {
		delete(attrs, HTMXAttrExt)
		delete(attrs, HTMXAttrSSEConnect)
		delete(attrs, HTMXAttrSSESwap)
		attrs[HTMX4AttrSSEConnect] = interaction.URL
		attrs[HTMXAttrTarget] = "#" + interaction.ID
		if interaction.Target != "" {
			attrs[HTMXAttrTarget] = interaction.Target
		}

		attrs[HTMXAttrSwap] = interaction.Swap
		if interaction.Swap == "" {
			attrs[HTMXAttrSwap] = string(SwapInnerHTML)
		}
	}

	return attrs
}
