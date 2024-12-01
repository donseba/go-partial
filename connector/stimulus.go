package connector

import "net/http"

type Stimulus struct {
	base
}

func NewStimulus(c *Config) Connector {
	return &Stimulus{
		base: base{
			config:       c,
			targetHeader: "X-Stimulus-Target",
			selectHeader: "X-Stimulus-Select",
			actionHeader: "X-Stimulus-Action",
		},
	}
}

func (s *Stimulus) RenderPartial(r *http.Request) bool {
	return r.Header.Get(s.targetHeader) != ""
}
