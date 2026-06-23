package connector

import "net/http"

type Unpoly struct {
	base
}

const (
	UnpolyHeaderTarget HeaderKey = "X-Up-Target"
	UnpolyHeaderSelect HeaderKey = "X-Up-Select"
	UnpolyHeaderAction HeaderKey = "X-Up-Action"
)

func NewUnpoly(c *Config) Connector {
	return &Unpoly{
		base: base{
			config:       c,
			targetHeader: UnpolyHeaderTarget.String(),
			selectHeader: UnpolyHeaderSelect.String(),
			actionHeader: UnpolyHeaderAction.String(),
		},
	}
}

func (u *Unpoly) RenderPartial(r *http.Request) bool {
	return r.Header.Get(u.targetHeader) != ""
}
