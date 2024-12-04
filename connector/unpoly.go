package connector

import "net/http"

type Unpoly struct {
	base
}

func NewUnpoly(c *Config) Connector {
	return &Unpoly{
		base: base{
			config:       c,
			targetHeader: "X-Up-Target",
			selectHeader: "X-Up-Select",
			actionHeader: "X-Up-Action",
		},
	}
}

func (u *Unpoly) RenderPartial(r *http.Request) bool {
	return r.Header.Get(u.targetHeader) != ""
}
