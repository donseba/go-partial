// Package slots provides experimental named regions backed by child partials.
package slots

import (
	"html/template"

	partial "github.com/donseba/go-partial"
)

type (
	config struct {
		Slots map[string]*partial.Partial
	}

	extensionKey struct{}
	nameKey      struct{}
)

// Set registers a child partial as a named slot on parent.
func Set(parent *partial.Partial, name string, child *partial.Partial) *partial.Partial {
	if parent == nil || name == "" || child == nil {
		return parent
	}

	cfg := slotConfig(parent)
	if cfg.Slots == nil {
		cfg.Slots = make(map[string]*partial.Partial)
	}
	cfg.Slots[name] = child
	parent.SetExtension(extensionKey{}, cfg)
	child.SetExtension(nameKey{}, name)
	parent.With(child)
	return parent
}

// Name returns the slot name associated with a partial, if it was registered as a slot.
func Name(p *partial.Partial) string {
	if p == nil {
		return ""
	}
	value, ok := p.Extension(nameKey{})
	if !ok {
		return ""
	}
	name, _ := value.(string)
	return name
}

// FuncMap returns template helpers for rendering slots.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"slot":    Slot,
		"hasSlot": HasSlot,
	}
}

// Renderer installs slot helpers bound to the active render context.
func Renderer() partial.Renderer {
	return partial.RendererHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			if ctx == nil {
				return ctx, nil
			}
			ctx.SetFunc("slot", func(name string) template.HTML {
				return Slot(name, ctx)
			})
			ctx.SetFunc("hasSlot", func(name string) bool {
				return HasSlot(name, ctx)
			})
			return ctx, nil
		},
	}
}

// Slot renders a named slot from a render context.
//
// go-doc:sig func(string) html/template.HTML
func Slot(name string, ctx ...*partial.RenderContext) template.HTML {
	if len(ctx) == 0 || ctx[0] == nil || ctx[0].Runtime == nil || name == "" {
		return ""
	}
	current := ctx[0]
	child, ok := resolve(current.Partial, name)
	if !ok {
		return ""
	}
	out, err := current.Runtime.RenderPartialWithFallback(child)
	if err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return out
}

// HasSlot reports whether a named slot exists in a render context.
//
// go-doc:sig func(string) bool
func HasSlot(name string, ctx ...*partial.RenderContext) bool {
	if len(ctx) == 0 || ctx[0] == nil || name == "" {
		return false
	}
	_, ok := resolve(ctx[0].Partial, name)
	return ok
}

func resolve(p *partial.Partial, name string) (*partial.Partial, bool) {
	cfg := slotConfig(p)
	if cfg.Slots == nil {
		return nil, false
	}
	child, ok := cfg.Slots[name]
	return child, ok
}

func slotConfig(p *partial.Partial) config {
	if p == nil {
		return config{}
	}
	value, ok := p.Extension(extensionKey{})
	if !ok {
		return config{}
	}
	cfg, _ := value.(config)
	return cfg
}
