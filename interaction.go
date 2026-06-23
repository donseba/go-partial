package partial

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/donseba/go-partial/connector"
)

type (
	InteractionRenderer func(ctx context.Context, p *Partial, data *Data, interaction connector.Interaction, attrs map[string]string) (template.HTML, error)

	InteractionConfig interface {
		Interaction() connector.Interaction
	}

	Interaction struct {
		interaction connector.Interaction
	}
)

const (
	SwapInnerHTML = string(connector.SwapInnerHTML)
	SwapOuterHTML = string(connector.SwapOuterHTML)
)

func Async(endpoint string) Interaction {
	return newInteraction(connector.InteractionAsync, "", endpoint)
}

func Reveal(endpoint string) Interaction {
	return newInteraction(connector.InteractionReveal, "", endpoint)
}

func Poll(endpoint string) Interaction {
	return newInteraction(connector.InteractionPoll, "", endpoint)
}

func Stream(endpoint string) Interaction {
	return newInteraction(connector.InteractionStream, "", endpoint)
}

func Prefetch(endpoint string) Interaction {
	return newInteraction(connector.InteractionPrefetch, "", endpoint)
}

func Island(name string, endpoint string) Interaction {
	return newInteraction(connector.InteractionIsland, name, endpoint)
}

func Refresh(endpoint string) Interaction {
	return newInteraction(connector.InteractionRefresh, "", endpoint)
}

func On(event string, endpoint string) Interaction {
	return newInteraction(connector.InteractionOn, event, endpoint).
		Trigger(event).
		From("body").
		Placeholder("")
}

func newInteraction(kind connector.InteractionKind, name string, endpoint string) Interaction {
	return Interaction{interaction: connector.Interaction{
		Kind:    kind,
		Name:    name,
		URL:     endpoint,
		Params:  make(map[string]string),
		Options: make(map[string]string),
	}}
}

func (i Interaction) Interaction() connector.Interaction {
	interaction := i.interaction
	if interaction.Params == nil {
		interaction.Params = make(map[string]string)
	}
	if interaction.Options == nil {
		interaction.Options = make(map[string]string)
	}
	interaction.URL = resolveInteractionURL(interaction.URL, interaction.Params)
	return normalizeInteraction(interaction)
}

func (i Interaction) ID(id string) Interaction {
	i.interaction.ID = id
	return i
}

func (i Interaction) Target(target string) Interaction {
	i.interaction.Target = target
	return i
}

func (i Interaction) Swap(swap string) Interaction {
	i.interaction.Swap = swap
	return i
}

func (i Interaction) Trigger(trigger string) Interaction {
	i.interaction.Trigger = trigger
	return i
}

func (i Interaction) From(source string) Interaction {
	if i.interaction.Options == nil {
		i.interaction.Options = make(map[string]string)
	}
	i.interaction.Options["from"] = source
	return i
}

func (i Interaction) Every(interval time.Duration) Interaction {
	i.interaction.Interval = interval.String()
	return i
}

func (i Interaction) EveryString(interval string) Interaction {
	i.interaction.Interval = interval
	return i
}

func (i Interaction) Placeholder(placeholder string) Interaction {
	i.interaction.Placeholder = placeholder
	if i.interaction.Options == nil {
		i.interaction.Options = make(map[string]string)
	}
	i.interaction.Options["placeholder"] = placeholder
	return i
}

func (i Interaction) Param(key string, value any) Interaction {
	if i.interaction.Params == nil {
		i.interaction.Params = make(map[string]string)
	}
	i.interaction.Params[key] = fmt.Sprint(value)
	return i
}

func (i Interaction) Option(key string, value any) Interaction {
	if i.interaction.Options == nil {
		i.interaction.Options = make(map[string]string)
	}
	i.interaction.Options[key] = fmt.Sprint(value)
	return i
}

func DefaultInteractionRenderer() InteractionRenderer {
	return func(ctx context.Context, p *Partial, data *Data, interaction connector.Interaction, attrs map[string]string) (template.HTML, error) {
		switch interaction.Kind {
		case connector.InteractionPrefetch:
			return template.HTML(`<link ` + renderInteractionAttrs(attrs) + `>`), nil
		case connector.InteractionRefresh:
			return template.HTML(`<button type="button" id="` + escapeAttr(interaction.ID) + `" ` + renderInteractionAttrs(attrs) + `>` + interactionPlaceholder(interaction) + `</button>`), nil
		default:
			element := "div"
			if _, ok := attrs["src"]; ok {
				if _, lazy := attrs["loading"]; lazy {
					element = "turbo-frame"
				}
			}
			return template.HTML(`<` + element + ` id="` + escapeAttr(interaction.ID) + `" ` + renderInteractionAttrs(attrs) + `>` + interactionPlaceholder(interaction) + `</` + element + `>`), nil
		}
	}
}

func renderInteractionAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}

	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	wrote := false
	for _, key := range keys {
		if key == "id" || strings.HasPrefix(key, "__") {
			continue
		}
		if wrote {
			b.WriteByte(' ')
		}
		b.WriteString(escapeAttr(key))
		b.WriteString(`="`)
		b.WriteString(escapeAttr(attrs[key]))
		b.WriteByte('"')
		wrote = true
	}
	return b.String()
}

func interactionPlaceholder(interaction connector.Interaction) string {
	return template.HTMLEscapeString(interaction.Placeholder)
}

func escapeAttr(value string) string {
	return template.HTMLEscapeString(value)
}
