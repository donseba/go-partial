// Package debug provides an optional renderer for debug output.
package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"reflect"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/internal/csp"
)

// RenderKindDebug is the renderer kind used for debug fragments.
const RenderKindDebug partial.RenderKind = "debug"

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[config]().PkgPath()

// FuncMap returns the optional debug template helper.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"debug": Debug,
	}
}

// Debug renders a diagnostic value through the active render stage chain.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, value any) html/template.HTML
//
// In a text render (partial.RenderText), such as the subject of an e-mail,
// it writes the value as plain text, without the HTML box.
func Debug(runtime *partial.Runtime, value any) template.HTML {
	if runtime == nil {
		return escapedDebugValue(value)
	}

	if ctx := runtime.RenderContext(); ctx != nil && ctx.Kind == partial.RenderKindText {
		// RenderText writes it unescaped.
		return template.HTML(FormatValue(value))
	}

	out, err := runtime.RenderWith(RenderKindDebug, "", value, func(ctx *partial.RenderContext) (template.HTML, error) {
		return escapedDebugValue(ctx.Data), nil
	})
	if err != nil {
		return escapedDebugValue(value)
	}
	return out
}

// Data is the template data used by the debug renderer.
type Data struct {
	Value     any
	Output    string
	PartialID string
	Templates []string
	Request   *http.Request
	URL       *url.URL
}

// style is the text of the <style> element of the debug box, everything
// between its tags, which StyleHash hashes. The box styles itself with its
// classes and this one element rather than with style attributes, which a
// Content-Security-Policy blocks unless it has 'unsafe-inline'. Each box
// carries the element, so it is styled wherever it is swapped in; the
// rules are the same each time.
const style = `
.go-partial-debug{background:#fff;border:1px solid #d8d5ca;border-left:4px solid #1f6f65;border-radius:8px;color:#252522;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;margin:12px 0;padding:14px}
.go-partial-debug__header{align-items:center;display:flex;gap:8px;justify-content:space-between;margin-bottom:10px}
.go-partial-debug__label{color:#174f49;font-size:13px;text-transform:uppercase}
.go-partial-debug__partial{color:#67645b;font-size:12px}
.go-partial-debug__value{background:#eeece4;border:1px solid #d8d5ca;border-radius:6px;color:#252522;font-family:ui-monospace,SFMono-Regular,Consolas,'Liberation Mono',Menlo,monospace;font-size:12px;line-height:1.45;margin:0;overflow:auto;padding:12px;white-space:pre-wrap}
`

const debugTemplate = `<section class="go-partial-debug" role="note">
<style>` + style + `</style>
<header class="go-partial-debug__header">
<strong class="go-partial-debug__label">go-partial debug</strong>
{{ if .PartialID }}<span class="go-partial-debug__partial">{{ .PartialID }}</span>{{ end }}
</header>
<pre class="go-partial-debug__value">{{ .Output }}</pre>
</section>`

// StyleHash returns the Content-Security-Policy hash source of the inline
// <style> element of the debug box, 'sha256-…' with its quotes, for the
// style-src directive of a policy without 'unsafe-inline', in development,
// where the debug stage is used:
//
//	style-src 'self' 'sha256-…'
//
// Without it, such a policy blocks the box's style and the box shows its
// value unstyled, in a <pre>. The hash changes only when go-partial changes
// the box's style.
func StyleHash() string {
	return csp.Hash(style)
}

type (
	// Option configures the debug stage.
	Option func(*config)

	config struct {
		bindFunc      bool
		unlessDefined bool
	}

	// FuncOption configures the debug helper of WithFunc.
	FuncOption func(*config)
)

// WithFunc makes the stage give templates the debug helper bound to the
// render, so they call {{ debug . }} without the runtime; {{ debug runtime . }}
// keeps working. Templates need no FuncMap for it. A debug function the
// application registers itself is then never called, unless the option
// UnlessDefined is given.
func WithFunc(opts ...FuncOption) Option {
	return func(cfg *config) {
		cfg.bindFunc = true

		for _, opt := range opts {
			if opt != nil {
				opt(cfg)
			}
		}
	}
}

// UnlessDefined makes WithFunc give templates the debug helper only where
// the application registered no debug function of its own with SetFunc,
// which is then called instead, without a warning; Disabled, of this
// package, does not count as one, so a root can register it for production
// and use the stage in development:
//
//	root.SetFunc(debug.Disabled()).SetFunc(siteFuncs)
//	if development {
//		root.Use(debug.Stage(debug.WithFunc(debug.UnlessDefined())))
//	}
func UnlessDefined() FuncOption {
	return func(cfg *config) {
		cfg.unlessDefined = true
	}
}

// Disabled returns a debug helper that renders nothing, for production:
// templates that call {{ debug . }} or {{ debug runtime . }} keep working
// and show nothing.
//
//	if development {
//		root.Use(debug.Stage(debug.WithFunc()))
//	} else {
//		root.SetFunc(debug.Disabled())
//	}
func Disabled() template.FuncMap {
	return template.FuncMap{
		// go-doc:sig func(value any) html/template.HTML
		"debug": disabled,
	}
}

func disabled(...any) template.HTML {
	return ""
}

// Stage returns a render stage that handles debug render contexts. With
// WithFunc, it also gives templates the debug helper.
func Stage(options ...Option) partial.RenderStage {
	var cfg config

	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	hooks := partial.RenderStageHooks{
		Package: packagePath,
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if ctx == nil || ctx.Kind != RenderKindDebug {
				return next(ctx)
			}

			tmpl, err := template.New("go-partial-debug").Parse(debugTemplate)
			if err != nil {
				return "", err
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, BuildData(ctx)); err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		},
	}

	if cfg.bindFunc {
		if cfg.unlessDefined {
			hooks.FallbackFuncNames = []string{"debug"}
		} else {
			hooks.FuncNames = []string{"debug"}
		}

		hooks.ResolveFunc = func(ctx *partial.RenderContext, name string) (any, bool) {
			if name != "debug" || ctx == nil {
				return nil, false
			}

			return func(args ...any) template.HTML {
				return debugArgs(ctx.Runtime, args)
			}, true
		}
	}

	return hooks
}

// debugArgs renders {{ debug . }} with the render's runtime, and
// {{ debug runtime . }} with the one given.
func debugArgs(runtime *partial.Runtime, args []any) template.HTML {
	switch len(args) {
	case 0:
		return Debug(runtime, nil)
	case 1:
		return Debug(runtime, args[0])
	case 2:
		if given, ok := args[0].(*partial.Runtime); ok {
			return Debug(given, args[1])
		}
	}

	return Debug(runtime, args)
}

// BuildData converts a render context into debug template data.
func BuildData(ctx *partial.RenderContext) Data {
	var value any
	if ctx != nil {
		value = ctx.Data
	}
	data := Data{
		Value:  value,
		Output: FormatValue(value),
	}
	if ctx != nil {
		data.Request = ctx.Request
		data.URL = ctx.URL
		if ctx.Partial != nil {
			data.PartialID = ctx.Partial.PartialID()
			data.Templates = ctx.Partial.TemplatePaths()
		}
	}
	return data
}

// FormatValue formats a debug value for display.
func FormatValue(value any) string {
	out, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		return string(out)
	}
	return fmt.Sprintf("%#v", value)
}

func escapedDebugValue(value any) template.HTML {
	return template.HTML(template.HTMLEscapeString(fmt.Sprintf("%#v", value)))
}
