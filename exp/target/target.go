// Package target provides experimental dynamic target resolution.
package target

import (
	"context"
	"html/template"
	"net/http"
	"reflect"
	"slices"

	partial "github.com/donseba/go-partial"
)

type (
	// Resolver resolves a connector target name to a partial.
	Resolver func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool)

	extensionKey struct{}
)

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[extensionKey]().PkgPath()

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
	if renderCtx == nil || renderCtx.Runtime == nil {
		return ""
	}
	return renderCtx.Runtime.Connector().GetTargetHeader()
}

// TargetValue returns the current target value from the request.
//
// go-doc:sig func() string
func TargetValue(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil || renderCtx.Runtime == nil {
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

// Stage gives templates the target helpers and applies configured target
// resolvers. The helpers are made only for the templates that call them.
func Stage() partial.RenderStage {
	return partial.RenderStageHooks{
		Package:     packagePath,
		FuncNames:   []string{"targetHeader", "targetValue", "targetIs"},
		ResolveFunc: resolveFunc,
		PrepareFunc: prepare,
	}
}

// resolveFunc gives a template that calls them the target helpers of its
// render.
func resolveFunc(ctx *partial.RenderContext, name string) (any, bool) {
	switch name {
	case "targetHeader":
		return func() string {
			return TargetHeader(ctx)
		}, true
	case "targetValue":
		return func() string {
			return TargetValue(ctx)
		}, true
	case "targetIs":
		return func(in ...string) bool {
			return targetIs(ctx, in...)
		}, true
	default:
		return nil, false
	}
}

// prepare answers a target request with the partial the tree's resolver
// returns for it, if any, unless an earlier stage resolved it.
func prepare(ctx *partial.RenderContext) (*partial.RenderContext, error) {
	if ctx == nil || ctx.Partial == nil || ctx.Kind != partial.RenderKindTarget || ctx.TargetResolved() {
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
	if !ok || resolved == nil {
		return ctx, nil
	}

	ctx.Partial = resolved

	return ctx, nil
}

func firstRenderContext(ctx []*partial.RenderContext) *partial.RenderContext {
	if len(ctx) == 0 {
		return nil
	}
	return ctx[0]
}
