// Package selection provides experimental selected-partial rendering helpers.
package selection

import (
	"fmt"
	"html/template"
	"net/http"
	"reflect"
	"slices"

	partial "github.com/donseba/go-partial"
)

type config struct {
	Default  string
	Partials map[string]*partial.Partial
}

type extensionKey struct{}

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[extensionKey]().PkgPath()

// WithSelectMap configures the named partials that the selection helper can render.
func WithSelectMap(p *partial.Partial, defaultKey string, partials map[string]*partial.Partial) *partial.Partial {
	if p == nil {
		return nil
	}
	return p.SetExtension(extensionKey{}, config{Default: defaultKey, Partials: partials})
}

// FuncMap returns placeholders for the selection template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"selection":       SelectionHTML,
		"selectionHeader": SelectionHeader,
		"selectionValue":  SelectionValue,
		"selectionIs":     SelectionIs,
	}
}

// SelectionHTML renders the selected partial for a render context.
//
// go-doc:sig func() html/template.HTML
func SelectionHTML(ctx ...*partial.RenderContext) template.HTML {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil {
		return ""
	}
	return render(renderCtx)
}

// SelectionHeader returns the connector selection header name.
//
// go-doc:sig func() string
func SelectionHeader(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil || renderCtx.Runtime == nil {
		return ""
	}
	return renderCtx.Runtime.Connector().GetSelectHeader()
}

// SelectionValue returns the selected key for a render context.
//
// go-doc:sig func() string
func SelectionValue(ctx ...*partial.RenderContext) string {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil {
		return ""
	}
	return selectionValue(renderCtx)()
}

// SelectionIs reports whether the selected key matches any provided value.
//
// go-doc:sig func(values ...string) bool
func SelectionIs(values ...string) bool {
	return selectionIs(nil, values...)
}

func selectionIs(ctx *partial.RenderContext, values ...string) bool {
	if ctx == nil {
		return false
	}
	return slices.Contains(values, SelectionValue(ctx))
}

// Stage gives templates the selection helpers, which render selected child
// partials. They are made only for the templates that call them.
func Stage() partial.RenderStage {
	return partial.RenderStageHooks{
		Package:   packagePath,
		FuncNames: []string{"selection", "selectionHeader", "selectionValue", "selectionIs"},
		ResolveFunc: func(ctx *partial.RenderContext, name string) (any, bool) {
			if ctx == nil || ctx.Partial == nil {
				return nil, false
			}

			switch name {
			case "selectionHeader":
				return func() string {
					return SelectionHeader(ctx)
				}, true
			case "selectionValue":
				return func() string {
					return SelectionValue(ctx)
				}, true
			case "selectionIs":
				return func(in ...string) bool {
					return selectionIs(ctx, in...)
				}, true
			case "selection":
				return func() template.HTML {
					return SelectionHTML(ctx)
				}, true
			default:
				return nil, false
			}
		},
	}
}

func firstRenderContext(ctx []*partial.RenderContext) *partial.RenderContext {
	if len(ctx) == 0 {
		return nil
	}
	return ctx[0]
}

func selectionConfig(ctx *partial.RenderContext) (config, bool) {
	value, ok := ctx.Partial.Extension(extensionKey{})
	if !ok {
		return config{}, false
	}
	cfg, ok := value.(config)
	return cfg, ok
}

func selectionValue(ctx *partial.RenderContext) func() string {
	return func() string {
		selected := ctx.Runtime.Connector().GetSelectValue(request(ctx))
		cfg, ok := selectionConfig(ctx)
		if !ok || selected != "" {
			return selected
		}
		return cfg.Default
	}
}

func render(ctx *partial.RenderContext) template.HTML {
	cfg, ok := selectionConfig(ctx)
	if !ok {
		return template.HTML("selection is not configured")
	}

	selected := ctx.Runtime.Connector().GetSelectValue(request(ctx))
	key := selected
	if key == "" {
		key = cfg.Default
	}

	selectedPartial := cfg.Partials[key]
	if selectedPartial == nil {
		return template.HTML(fmt.Sprintf("selected partial '%s' not found in parent '%s'", key, ctx.Partial.PartialID()))
	}

	html, err := ctx.Runtime.RenderPartialWithFallback(selectedPartial)
	if err != nil {
		return template.HTML(fmt.Sprintf("error rendering selected partial '%s': %v", key, err))
	}
	return html
}

func request(ctx *partial.RenderContext) *http.Request {
	if ctx == nil || ctx.Request == nil {
		return &http.Request{}
	}
	return ctx.Request
}
