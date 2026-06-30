// Package target provides experimental dynamic target resolution.
package target

import (
	"context"
	"html/template"
	"net/http"
	"slices"

	partial "github.com/donseba/go-partial"
)

type (
	// Resolver resolves a connector target name to a partial.
	Resolver func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool)

	extensionKey struct{}
)

// WithResolver configures a partial-level target resolver.
func WithResolver(p *partial.Partial, resolver Resolver) *partial.Partial {
	if p == nil {
		return nil
	}
	return p.SetExtension(extensionKey{}, resolver)
}

// FuncMap returns placeholders for target template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"targetHeader": TargetHeader,
		"targetValue":  TargetValue,
		"targetIs":     TargetIs,
	}
}

// TargetHeader returns the connector target header name.
//
// go-doc:sig func() string
func TargetHeader(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil || renderCtx.Runtime == nil || renderCtx.Runtime.Connector() == nil {
		return ""
	}
	return renderCtx.Runtime.Connector().GetTargetHeader()
}

// TargetValue returns the current target value from the request.
//
// go-doc:sig func() string
func TargetValue(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil || renderCtx.Runtime == nil || renderCtx.Runtime.Connector() == nil {
		return ""
	}
	return renderCtx.Runtime.Connector().GetTargetValue(renderCtx.Request)
}

// TargetIs reports whether the current target matches any provided value.
//
// go-doc:sig func(values ...string) bool
func TargetIs(values ...string) bool {
	return targetIs(nil, values...)
}

func targetIs(ctx *partial.RenderContext, values ...string) bool {
	if ctx == nil {
		return false
	}
	return slices.Contains(values, TargetValue(ctx))
}

// Renderer installs target helpers and applies configured target resolvers.
func Renderer() partial.Renderer {
	return partial.RendererHooks{
		PreflightFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			if ctx == nil || ctx.Partial == nil {
				return ctx, nil
			}

			ctx.SetFunc("targetHeader", func() string { return TargetHeader(ctx) })
			ctx.SetFunc("targetValue", func() string { return TargetValue(ctx) })
			ctx.SetFunc("targetIs", func(in ...string) bool {
				return targetIs(ctx, in...)
			})

			if ctx.Kind != partial.RenderKindTarget {
				return ctx, nil
			}

			value, ok := ctx.Partial.Extension(extensionKey{})
			if !ok {
				return ctx, nil
			}
			resolver, ok := value.(Resolver)
			if !ok || resolver == nil {
				return ctx, nil
			}
			resolved, ok := resolver(ctx.Context, ctx.Request, ctx.Name)
			if ok && resolved != nil {
				ctx.Partial = resolved
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
