// Package debug provides an optional renderer for debug output.
package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	partial "github.com/donseba/go-partial"
)

// RenderKindDebug is the renderer kind used for debug fragments.
const RenderKindDebug partial.RenderKind = "debug"

// FuncMap returns the optional debug template helper.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"debug": Debug,
	}
}

// Debug renders a diagnostic value through the active renderer chain.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, value any) html/template.HTML
func Debug(runtime *partial.Runtime, value any) template.HTML {
	if runtime == nil {
		return escapedDebugValue(value)
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

const debugTemplate = `<section class="go-partial-debug" role="note" style="background:#fff;border:1px solid #d8d5ca;border-left:4px solid #1f6f65;border-radius:8px;color:#252522;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;margin:12px 0;padding:14px">
<header style="align-items:center;display:flex;gap:8px;justify-content:space-between;margin-bottom:10px">
<strong style="color:#174f49;font-size:13px;text-transform:uppercase">go-partial debug</strong>
{{ if .PartialID }}<span style="color:#67645b;font-size:12px">{{ .PartialID }}</span>{{ end }}
</header>
<pre style="background:#eeece4;border:1px solid #d8d5ca;border-radius:6px;color:#252522;font-family:ui-monospace,SFMono-Regular,Consolas,'Liberation Mono',Menlo,monospace;font-size:12px;line-height:1.45;margin:0;overflow:auto;padding:12px;white-space:pre-wrap">{{ .Output }}</pre>
</section>`

// Renderer returns a renderer that handles debug render contexts.
func Renderer() partial.Renderer {
	return partial.RendererHooks{
		InFlightFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
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
