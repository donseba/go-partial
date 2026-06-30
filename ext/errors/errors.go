// Package errors provides an optional renderer for render failures.
package errors

import (
	"bytes"
	stderrors "errors"
	"html/template"
	"net/http"
	"net/url"
	"regexp"

	partial "github.com/donseba/go-partial"
)

// RenderKindError is the renderer kind used for error fragments.
const RenderKindError partial.RenderKind = "error"

type (
	// Mode controls how much error detail is rendered.
	Mode int

	// Data is the template data used by the error renderer.
	Data struct {
		Error         error
		Message       string
		PartialID     string
		Templates     []string
		Request       *http.Request
		URL           *url.URL
		Location      string
		Detailed      bool
		TemplateLabel string
	}

	config struct {
		mode Mode
	}

	// Option configures the error renderer.
	Option func(*config)

	stateKey struct{}
)

const (
	// ModeSafe hides detailed error messages from rendered output.
	ModeSafe Mode = iota
	// ModeDetailed includes diagnostic error details in rendered output.
	ModeDetailed
)

var templateLocationPattern = regexp.MustCompile(`template:\s+([^:]+:\d+(?::\d+)?)`)

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Template render error</title>
<style>
body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f7f4;color:#252522}
main{max-width:1040px;margin:0 auto;padding:32px 20px}
section{background:#fff;border:1px solid #d9d7cf;border-radius:8px;padding:20px;box-shadow:0 1px 2px rgba(0,0,0,.04)}
h1{font-size:24px;margin:0 0 8px}
p{margin:0 0 16px;color:#55524a}
dl{display:grid;grid-template-columns:120px 1fr;gap:8px 14px;margin:18px 0}
dt{font-weight:700;color:#3d3b35}
dd{margin:0;min-width:0;overflow-wrap:anywhere}
pre{white-space:pre-wrap;overflow:auto;background:#f2f0e8;border:1px solid #d8d5ca;color:#252522;border-radius:6px;padding:16px;font-size:13px;line-height:1.45}
</style>
</head>
<body>
<main>
<section>
<h1>Template render error</h1>
<p>The error response was rendered by go-partial because a template failed.</p>
<dl>
<dt>Partial ID</dt><dd>{{ .PartialID }}</dd>
<dt>{{ .TemplateLabel }}</dt><dd>{{ range $i, $template := .Templates }}{{ if $i }}, {{ end }}{{ $template }}{{ end }}</dd>
{{ if .URL }}<dt>URL</dt><dd>{{ .URL.String }}</dd>{{ end }}
{{ if .Detailed }}
{{ if .Location }}<dt>Location</dt><dd>{{ .Location }}</dd>{{ end }}
<dt>Error</dt><dd>{{ .Message }}</dd>
{{ end }}
</dl>
</section>
</main>
</body>
</html>`

const fragmentTemplate = `<section class="go-partial-error" role="alert" style="background:#fff;border:1px solid #d8d5ca;border-left:4px solid #8a4b12;border-radius:8px;padding:16px;color:#252522;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif">
<h1 style="font-size:20px;margin:0 0 8px">Template render error</h1>
<p style="margin:0 0 12px;color:#55524a">go-partial rendered this error response because a template failed during a partial request.</p>
<dl style="display:grid;grid-template-columns:110px 1fr;gap:8px 12px;margin:0 0 12px">
<dt style="font-weight:700">Partial ID</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .PartialID }}</dd>
<dt style="font-weight:700">{{ .TemplateLabel }}</dt><dd style="margin:0;overflow-wrap:anywhere">{{ range $i, $template := .Templates }}{{ if $i }}, {{ end }}{{ $template }}{{ end }}</dd>
{{ if .URL }}<dt style="font-weight:700">URL</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .URL.String }}</dd>{{ end }}
{{ if .Detailed }}
{{ if .Location }}<dt style="font-weight:700">Location</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .Location }}</dd>{{ end }}
<dt style="font-weight:700">Error</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .Message }}</dd>
{{ end }}
</dl>
</section>`

// WithMode configures the error detail mode.
func WithMode(mode Mode) Option {
	return func(cfg *config) {
		cfg.mode = mode
	}
}

// Renderer returns a renderer that handles error render contexts.
func Renderer(options ...Option) partial.Renderer {
	cfg := config{mode: ModeSafe}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return partial.RendererHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			if ctx == nil || ctx.Kind != RenderKindError {
				return ctx, nil
			}

			if ctx.Response == nil {
				ctx.Response = &partial.RenderResponse{Headers: make(map[string]string)}
			}
			if ctx.Response.Headers == nil {
				ctx.Response.Headers = make(map[string]string)
			}
			ctx.Response.Headers["Content-Type"] = "text/html; charset=utf-8"
			ctx.Response.Status = http.StatusInternalServerError
			if ctx.Name == "fragment" {
				ctx.Response.Status = http.StatusOK
			}

			data := BuildData(ctx, cfg.mode)
			ctx.Error = data.Error
			ctx.Data = data
			if ctx.Values == nil {
				ctx.Values = make(partial.RenderValues)
			}
			ctx.Values.Set(stateKey{}, data)
			return ctx, nil
		},
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if ctx == nil || ctx.Kind != RenderKindError {
				return next(ctx)
			}

			data := stateData(ctx, cfg.mode)
			tmplSource := pageTemplate
			if ctx.Name == "fragment" {
				tmplSource = fragmentTemplate
			}

			tmpl, err := template.New("go-partial-error").Parse(tmplSource)
			if err != nil {
				return "", err
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		},
		FinalizeFunc: func(ctx *partial.RenderContext, out template.HTML, renderErr error) (template.HTML, error) {
			if ctx == nil || ctx.Kind != RenderKindError || renderErr == nil {
				return out, renderErr
			}

			data := stateData(ctx, cfg.mode)
			if data.Error == nil {
				return out, renderErr
			}
			return out, stderrors.Join(renderErr, data.Error)
		},
	}
}

func stateData(ctx *partial.RenderContext, mode Mode) Data {
	if ctx == nil {
		return BuildData(nil, mode)
	}
	if ctx.Values != nil {
		if data, ok := ctx.Values.Get(stateKey{}).(Data); ok {
			return data
		}
	}
	if data, ok := ctx.Data.(Data); ok {
		return data
	}
	return BuildData(ctx, mode)
}

// BuildData converts a render context into error template data.
func BuildData(ctx *partial.RenderContext, mode Mode) Data {
	var err error
	if ctx != nil {
		err = ctx.Error
		if err == nil {
			err, _ = ctx.Data.(error)
		}
	}
	if err == nil {
		err = stderrors.New("render error")
	}

	data := Data{
		Error:    err,
		Message:  err.Error(),
		Location: ExtractTemplateLocation(err),
		Detailed: mode == ModeDetailed,
	}
	if ctx != nil {
		data.Request = ctx.Request
		data.URL = ctx.URL
		if ctx.Partial != nil {
			data.PartialID = ctx.Partial.PartialID()
			data.Templates = ctx.Partial.TemplatePaths()
		}
	}
	data.TemplateLabel = "Templates"
	if len(data.Templates) == 1 {
		data.TemplateLabel = "Template"
	}
	return data
}

// ExtractTemplateLocation extracts a template file and line from an error message.
func ExtractTemplateLocation(err error) string {
	if err == nil {
		return ""
	}
	match := templateLocationPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return ""
	}
	return match[1]
}
