package connector

import "net/http"

type AlpineAjax struct {
	base
}

func NewAlpineAjax(c *Config) Connector {
	return &AlpineAjax{
		base: base{
			config:       c,
			targetHeader: "X-Alpine-Target",
			selectHeader: "X-Alpine-Select",
			actionHeader: "X-Alpine-Action",
		},
	}
}

func (a *AlpineAjax) RenderPartial(r *http.Request) bool {
	return r.Header.Get(a.targetHeader) != ""
}
