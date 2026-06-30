// Package actions provides experimental render-time action hooks for partials.
package actions

import (
	"context"
	"fmt"
	"html/template"
	"slices"

	partial "github.com/donseba/go-partial"
)

type (
	// Action can replace or render a partial during a request-aware render.
	Action func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error)

	config struct {
		action         Action
		templateAction Action
	}

	extensionKey struct{}
)

// WithAction configures a partial-level action that may replace the partial
// before its template is rendered.
func WithAction(p *partial.Partial, action Action) *partial.Partial {
	cfg := getConfig(p)
	cfg.action = action
	return p.SetExtension(extensionKey{}, cfg)
}

// WithTemplateAction configures the action template helper for a partial.
func WithTemplateAction(p *partial.Partial, action Action) *partial.Partial {
	cfg := getConfig(p)
	cfg.templateAction = action
	return p.SetExtension(extensionKey{}, cfg)
}

// FuncMap returns placeholders for the action template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"action":       ActionHTML,
		"actionHeader": ActionHeader,
		"actionValue":  ActionValue,
		"actionIs":     ActionIs,
	}
}

// ActionHTML renders the configured template action for a render context.
//
// go-doc:sig func() html/template.HTML
func ActionHTML(ctx ...*partial.RenderContext) template.HTML {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil {
		return ""
	}
	return renderTemplateAction(renderCtx)
}

// ActionHeader returns the connector action header name.
//
// go-doc:sig func() string
func ActionHeader(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil || renderCtx.Runtime == nil || renderCtx.Runtime.Connector() == nil {
		return ""
	}
	return renderCtx.Runtime.Connector().GetActionHeader()
}

// ActionValue returns the selected action value from the current request.
//
// go-doc:sig func() string
func ActionValue(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil || renderCtx.Runtime == nil || renderCtx.Runtime.Connector() == nil {
		return ""
	}
	return renderCtx.Runtime.Connector().GetActionValue(renderCtx.Request)
}

// ActionIs reports whether the current action value matches any provided value.
//
// go-doc:sig func(values ...string) bool
func ActionIs(values ...string) bool {
	return actionIs(nil, values...)
}

func actionIs(ctx *partial.RenderContext, values ...string) bool {
	if ctx == nil {
		return false
	}
	return slices.Contains(values, ActionValue(ctx))
}

// Renderer installs action helpers and executes configured partial actions.
func Renderer() partial.Renderer {
	return partial.RendererHooks{
		PreflightFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			if ctx == nil || ctx.Partial == nil {
				return ctx, nil
			}

			ctx.SetFunc("actionHeader", func() string { return ActionHeader(ctx) })
			ctx.SetFunc("actionValue", func() string { return ActionValue(ctx) })
			ctx.SetFunc("actionIs", func(in ...string) bool {
				return actionIs(ctx, in...)
			})
			ctx.SetFunc("action", func() template.HTML { return ActionHTML(ctx) })

			cfg := getConfig(ctx.Partial)
			if cfg.action == nil || ctx.Kind != partial.RenderKindPartial {
				return ctx, nil
			}
			nextPartial, err := cfg.action(ctx.Context, ctx.Partial, ctx.Runtime)
			if err != nil {
				return ctx, fmt.Errorf("error in action function: %w", err)
			}
			if nextPartial != nil {
				ctx.Partial = nextPartial
			}
			return ctx, nil
		},
	}
}

func firstRenderContext(ctx []*partial.RenderContext) *partial.RenderContext {
	if len(ctx) == 0 {
		return nil
	}
	return ctx[0]
}

func getConfig(p *partial.Partial) config {
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

func renderTemplateAction(ctx *partial.RenderContext) template.HTML {
	cfg := getConfig(ctx.Partial)
	if cfg.templateAction == nil {
		return template.HTML(fmt.Sprintf("no action callback found in partial '%s'", ctx.Partial.PartialID()))
	}
	actionPartial, err := cfg.templateAction(ctx.Context, ctx.Partial, ctx.Runtime)
	if err != nil {
		return template.HTML(fmt.Sprintf("error in action function: %v", err))
	}
	html, err := ctx.Runtime.RenderPartialWithFallback(actionPartial)
	if err != nil {
		return template.HTML(fmt.Sprintf("error rendering action partial: %v", err))
	}
	return html
}
