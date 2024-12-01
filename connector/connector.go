package connector

import "net/http"

type (
	Connector interface {
		RenderPartial(r *http.Request) bool
		GetTargetValue(r *http.Request) string
		GetSelectValue(r *http.Request) string
		GetActionValue(r *http.Request) string

		GetTargetHeader() string
		GetSelectHeader() string
		GetActionHeader() string
	}

	Config struct {
		UseURLQuery bool
	}

	base struct {
		config       *Config
		targetHeader string
		selectHeader string
		actionHeader string
	}
)

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
