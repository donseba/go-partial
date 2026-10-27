package connector

import "net/http"

type (
	HeaderKey string

	Connector interface {
		RenderPartial(r *http.Request) bool
		GetTargetValue(r *http.Request) string
		GetSelectValue(r *http.Request) string
		GetActionValue(r *http.Request) string

		GetTargetHeader() string
		GetSelectHeader() string
		GetActionHeader() string
		InteractionAttrs(interaction Interaction) map[string]string
		ResponseHeaders(response Response) map[string]string
	}

	Config struct {
		UseURLQuery bool
	}

	InteractionKind string

	Interaction struct {
		Kind        InteractionKind
		ID          string
		URL         string
		Target      string
		Swap        string
		Trigger     string
		Interval    string
		Placeholder string
		Name        string
		Params      map[string]string
		Options     map[string]string
		// Select picks the part of the response to swap, such as htmx's
		// hx-select, for connectors that can.
		Select string
		// Attrs are attributes written as given, such as hx-indicator or
		// hx-push-url, after the connector's own: they replace those of the
		// same name.
		Attrs map[string]string
	}

	// The Options of an interaction that connectors write in their own
	// syntax, where their client library can, such as hx-include,
	// hx-push-url and hx-replace-url for htmx.
	//
	//	OptionInclude:    a selector of elements whose values the request sends
	//	OptionPushURL:    "true", or the URL pushed to the browser history
	//	OptionReplaceURL: "true", or the URL that replaces the current one
	//	OptionMethod:     the method of a submit interaction
	//	OptionDelay:      the pause in typing a live interaction waits for

	base struct {
		config       *Config
		targetHeader string
		selectHeader string
		actionHeader string
	}
)

const (
	InteractionAsync    InteractionKind = "async"
	InteractionReveal   InteractionKind = "reveal"
	InteractionPoll     InteractionKind = "poll"
	InteractionStream   InteractionKind = "stream"
	InteractionPrefetch InteractionKind = "prefetch"
	InteractionRefresh  InteractionKind = "refresh"
	InteractionOn       InteractionKind = "on"
	// InteractionSubmit submits a form and swaps the response into the
	// target, such as htmx's hx-post. Options["method"] picks another
	// method, such as "put" or "delete", where the connector can.
	InteractionSubmit InteractionKind = "submit"
	// InteractionLive requests the URL as the user types, such as a live
	// search: Options["delay"] waits for a pause in typing, 300ms by
	// default.
	InteractionLive InteractionKind = "live"

	OptionInclude    = "include"
	OptionPushURL    = "pushURL"
	OptionReplaceURL = "replaceURL"
	OptionMethod     = "method"
	OptionDelay      = "delay"

	HeaderTarget HeaderKey = "X-Target"
	HeaderSelect HeaderKey = "X-Select"
	HeaderAction HeaderKey = "X-Action"

	HeaderLocation           HeaderKey = "X-Location"
	HeaderPushURL            HeaderKey = "X-Push-Url"
	HeaderRedirect           HeaderKey = "X-Redirect"
	HeaderRefresh            HeaderKey = "X-Refresh"
	HeaderReplaceURL         HeaderKey = "X-Replace-Url"
	HeaderReswap             HeaderKey = "X-Reswap"
	HeaderRetarget           HeaderKey = "X-Retarget"
	HeaderReselect           HeaderKey = "X-Reselect"
	HeaderTrigger            HeaderKey = "X-Trigger"
	HeaderTriggerAfterSettle HeaderKey = "X-Trigger-After-Settle"
	HeaderTriggerAfterSwap   HeaderKey = "X-Trigger-After-Swap"
)

func (h HeaderKey) String() string {
	return string(h)
}

func (x *base) RenderPartial(r *http.Request) bool {
	if r == nil {
		return false
	}
	return headerValue(r.Header, x.targetHeader) != ""
}

func (x *base) GetTargetHeader() string {
	return x.targetHeader
}

func (x *base) GetSelectHeader() string {
	return x.selectHeader
}

func (x *base) GetActionHeader() string {
	return x.actionHeader
}

func (x *base) InteractionAttrs(interaction Interaction) map[string]string {
	attrs := map[string]string{
		"data-partial-interaction": string(interaction.Kind),
		"data-url":                 interaction.URL,
	}
	if interaction.Target != "" {
		attrs["data-target"] = interaction.Target
	}
	if interaction.Interval != "" {
		attrs["data-interval"] = interaction.Interval
	}
	if interaction.Trigger != "" {
		attrs["data-trigger"] = interaction.Trigger
	}
	setAttr(attrs, "data-select", interaction.Select)
	return withInteractionAttrs(attrs, interaction)
}

// withInteractionAttrs adds the interaction's own attributes to attrs,
// replacing those of the same name.
func withInteractionAttrs(attrs map[string]string, interaction Interaction) map[string]string {
	for name, value := range interaction.Attrs {
		if name == "" {
			continue
		}

		attrs[name] = value
	}

	return attrs
}

func (x *base) GetTargetValue(r *http.Request) string {
	if r == nil {
		return ""
	}

	if targetValue := headerValue(r.Header, x.targetHeader); targetValue != "" {
		return targetValue
	}

	if x.config.useURLQuery() && r.URL != nil {
		return r.URL.Query().Get("target")
	}

	return ""
}

func (x *base) GetSelectValue(r *http.Request) string {
	if r == nil {
		return ""
	}

	if selectValue := headerValue(r.Header, x.selectHeader); selectValue != "" {
		return selectValue
	}

	if x.config.useURLQuery() && r.URL != nil {
		return r.URL.Query().Get("select")
	}

	return ""
}

func (x *base) GetActionValue(r *http.Request) string {
	if r == nil {
		return ""
	}

	if actionValue := headerValue(r.Header, x.actionHeader); actionValue != "" {
		return actionValue
	}

	if x.config.useURLQuery() && r.URL != nil {
		return r.URL.Query().Get("action")
	}

	return ""
}

func (c *Config) useURLQuery() bool {
	if c == nil {
		return false
	}

	return c.UseURLQuery
}
