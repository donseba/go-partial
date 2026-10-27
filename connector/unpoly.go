package connector

import (
	"net/http"
	"strconv"
	"time"
)

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
	return headerValue(r.Header, u.targetHeader) != ""
}

func (u *Unpoly) InteractionAttrs(interaction Interaction) map[string]string {
	attrs := map[string]string{}
	target := "#" + interaction.ID
	if interaction.Target != "" {
		target = interaction.Target
	}

	if target == TargetSelf {
		// Unpoly calls the element an interaction is placed on its origin.
		target = ":origin"
	}

	switch interaction.Kind {
	case InteractionPrefetch:
		attrs["rel"] = "prefetch"
		attrs["href"] = interaction.URL
	case InteractionSubmit:
		// Unpoly submits the form to its own action.
		attrs["up-submit"] = ""
		attrs[UnpolyAttrTarget] = target
	case InteractionLive:
		// Unpoly submits the input's form as the user types.
		attrs["up-autosubmit"] = ""
		attrs["up-watch-delay"] = milliseconds(liveDelay(interaction))
		attrs[UnpolyAttrTarget] = target
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
	return withInteractionAttrs(attrs, interaction)
}

// milliseconds writes a duration such as 300ms as Unpoly's number of
// milliseconds; a value that is not a duration is written as it is.
func milliseconds(value string) string {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return value
	}

	return strconv.FormatInt(duration.Milliseconds(), 10)
}
