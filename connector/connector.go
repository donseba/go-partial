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
	}

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
	return r.Header.Get(x.targetHeader) != ""
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
	return attrs
}

func (x *base) GetTargetValue(r *http.Request) string {
	if targetValue := r.Header.Get(x.targetHeader); targetValue != "" {
		return targetValue
	}

	if x.config.useURLQuery() {
		return r.URL.Query().Get("target")
	}

	return ""
}

func (x *base) GetSelectValue(r *http.Request) string {
	if selectValue := r.Header.Get(x.selectHeader); selectValue != "" {
		return selectValue
	}

	if x.config.useURLQuery() {
		return r.URL.Query().Get("select")
	}

	return ""
}

func (x *base) GetActionValue(r *http.Request) string {
	if actionValue := r.Header.Get(x.actionHeader); actionValue != "" {
		return actionValue
	}

	if x.config.useURLQuery() {
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
