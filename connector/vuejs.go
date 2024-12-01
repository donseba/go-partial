package connector

import "net/http"

type Vue struct {
	base
}

func NewVue(c *Config) Connector {
	return &Vue{
		base: base{
			config:       c,
			targetHeader: "X-Vue-Target",
			selectHeader: "X-Vue-Select",
			actionHeader: "X-Vue-Action",
		},
	}
}

func (v *Vue) RenderPartial(r *http.Request) bool {
	return r.Header.Get(v.targetHeader) != ""
}
