package connector

import "net/http"

type Alpine struct {
	base
}

func NewAlpine(c *Config) Connector {
	return &Alpine{
		base: base{
			config:       c,
			targetHeader: "X-Alpine-Target",
			selectHeader: "X-Alpine-Select",
			actionHeader: "X-Alpine-Action",
		},
	}
}

func (a *Alpine) RenderPartial(r *http.Request) bool {
	return r.Header.Get(a.targetHeader) != ""
}
