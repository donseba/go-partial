package connector

import "net/http"

type Unpoly struct {
	base
}

const (
	UnpolyHeaderTarget HeaderKey = "X-Up-Target"
	UnpolyHeaderSelect HeaderKey = "X-Up-Select"
	UnpolyHeaderAction HeaderKey = "X-Up-Action"

	UnpolyAttrGet     = "up-get"
	UnpolyAttrTrigger = "up-trigger"
	UnpolyAttrTarget  = "up-target"
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
	if r == nil {
		return false
	}
	return r.Header.Get(u.targetHeader) != ""
}

func (u *Unpoly) InteractionAttrs(interaction Interaction) map[string]string {
	attrs := map[string]string{}
	target := "#" + interaction.ID
	if interaction.Target != "" {
		target = interaction.Target
	}
	switch interaction.Kind {
	case InteractionPrefetch:
		attrs["rel"] = "prefetch"
		attrs["href"] = interaction.URL
	default:
		attrs[UnpolyAttrGet] = interaction.URL
		attrs[UnpolyAttrTarget] = target
		if interaction.Kind == InteractionReveal {
			attrs[UnpolyAttrTrigger] = "reveal"
		} else if interaction.Kind == InteractionPoll && interaction.Interval != "" {
			attrs[UnpolyAttrTrigger] = "every " + interaction.Interval
		} else if interaction.Trigger != "" {
			attrs[UnpolyAttrTrigger] = interaction.Trigger
		} else {
			attrs[UnpolyAttrTrigger] = "load"
		}
	}
	return attrs
}
