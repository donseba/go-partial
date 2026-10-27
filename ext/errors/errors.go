// Package errors provides an optional renderer for render failures.
package errors

import (
	"bytes"
	"context"
	stderrors "errors"
	"html/template"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/exp/flash"
)

// RenderKindError is the renderer kind used for error fragments.
const RenderKindError partial.RenderKind = "error"

// EventTemplateFailed is emitted, as a warning, when a custom error template
// (see WithTemplate) fails and the built-in markup answers instead.
const EventTemplateFailed = "ext.errors.template_failed"

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[stateKey]().PkgPath()

type (
	// Mode controls how much error detail is rendered.
	Mode int

	// Data is the template data used by the error renderer.
	//
	// Message is the error's message in ModeDetailed, and SafeMessage in
	// ModeSafe, so an error template can show it in both; WithMessage
	// chooses another. Error and Location tell what failed: show them only
	// when Detailed is true. Status is the status of the error: the one
	// given to Respond, or 500 for a render that failed. Title is the
	// heading of the page, the status text of Status (http.StatusText) or
	// the text of WithTitle. PartialID and Templates name the partial that
	// failed to render, in ModeDetailed only; they are empty in ModeSafe,
	// and for an error of Respond, which happened outside a render. Lang is
	// the language of the page, the text of WithLang, which the built-in
	// page writes as its lang attribute, en when it is empty.
	Data struct {
		Error         error
		Message       string
		Title         string
		Lang          string
		Status        int
		PartialID     string
		Templates     []string
		Request       *http.Request
		URL           *url.URL
		Location      string
		Detailed      bool
		TemplateLabel string
	}

	config struct {
		mode     Mode
		page     *partial.Partial
		fragment *partial.Partial
		// flash answers failed partial requests with an error flash
		// message; see WithFlash.
		flash        bool
		flashMessage func(ctx *partial.RenderContext, data Data) string
		// message chooses Data.Message; see WithMessage.
		message func(ctx *partial.RenderContext, data Data) string
		// title chooses Data.Title; see WithTitle.
		title func(ctx *partial.RenderContext, data Data) string
		// lang chooses Data.Lang; see WithLang.
		lang func(ctx *partial.RenderContext, data Data) string
	}

	// Option configures the error renderer.
	Option func(*config)

	stateKey struct{}

	// renderingKey marks the context of a render of a custom error
	// template with its Data, so a failure inside it is answered with the
	// built-in markup, and its functions can tell (see FromContext).
	renderingKey struct{}
)

const (
	// ModeSafe hides detailed error messages from rendered output.
	ModeSafe Mode = iota
	// ModeDetailed includes diagnostic error details in rendered output.
	ModeDetailed
)

// SafeMessage is Data.Message in ModeSafe: it tells nothing of the error,
// which is for the log.
const SafeMessage = "Something went wrong. Please try again."

var templateLocationPattern = regexp.MustCompile(`template:\s+([^:]+:\d+(?::\d+)?)`)

// pageStyle is the text of the <style> element of the built-in error page,
// everything between its tags, which StyleHash hashes. The page keeps its
// style inline, rather than linking a stylesheet, so it looks right when
// the application's own assets are what failed.
const pageStyle = `
body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f7f4;color:#252522}
main{max-width:1040px;margin:0 auto;padding:32px 20px}
section{background:#fff;border:1px solid #d9d7cf;border-radius:8px;padding:20px;box-shadow:0 1px 2px rgba(0,0,0,.04)}
h1{font-size:24px;margin:0 0 8px}
p{margin:0 0 16px;color:#55524a}
dl{display:grid;grid-template-columns:120px 1fr;gap:8px 14px;margin:18px 0}
dt{font-weight:700;color:#3d3b35}
dd{margin:0;min-width:0;overflow-wrap:anywhere}
pre{white-space:pre-wrap;overflow:auto;background:#f2f0e8;border:1px solid #d8d5ca;color:#252522;border-radius:6px;padding:16px;font-size:13px;line-height:1.45}
`

const pageTemplate = `<!doctype html>
<html lang="{{ with .Lang }}{{ . }}{{ else }}en{{ end }}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ if and .Detailed (or .PartialID .Templates) }}Template render error{{ else }}{{ template "title" . }}{{ end }}</title>
<style>` + pageStyle + `</style>
</head>
<body>
<main>
<section>
{{ if and .Detailed (or .PartialID .Templates) }}
<h1>Template render error</h1>
<p>The error response was rendered by go-partial because a template failed.</p>
{{ else }}
<h1>{{ template "title" . }}</h1>
{{ if not .Detailed }}<p>{{ .Message }}</p>{{ end }}
{{ end }}
<dl>
{{ if and .Detailed (or .PartialID .Templates) }}
<dt>Partial ID</dt><dd>{{ .PartialID }}</dd>
<dt>{{ .TemplateLabel }}</dt><dd>{{ range $i, $template := .Templates }}{{ if $i }}, {{ end }}{{ $template }}{{ end }}</dd>
{{ end }}
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

// fragmentTemplate is the built-in error fragment. It has no style of its
// own, neither a <style> element nor style attributes, which a
// Content-Security-Policy without 'unsafe-inline' blocks: its classes are
// styled by the stylesheet of StylesheetFS, or by the application's own
// CSS, and unstyled it reads as a heading, a paragraph and a list.
const fragmentTemplate = `<section class="go-partial-error" role="alert">
{{ if and .Detailed (or .PartialID .Templates) }}
<h1 class="go-partial-error__title">Template render error</h1>
<p class="go-partial-error__message">go-partial rendered this error response because a template failed during a partial request.</p>
{{ else }}
<h1 class="go-partial-error__title">{{ template "title" . }}</h1>
{{ if not .Detailed }}<p class="go-partial-error__message">{{ .Message }}</p>{{ end }}
{{ end }}
<dl class="go-partial-error__details">
{{ if and .Detailed (or .PartialID .Templates) }}
<dt>Partial ID</dt><dd>{{ .PartialID }}</dd>
<dt>{{ .TemplateLabel }}</dt><dd>{{ range $i, $template := .Templates }}{{ if $i }}, {{ end }}{{ $template }}{{ end }}</dd>
{{ end }}
{{ if .URL }}<dt>URL</dt><dd>{{ .URL.String }}</dd>{{ end }}
{{ if .Detailed }}
{{ if .Location }}<dt>Location</dt><dd>{{ .Location }}</dd>{{ end }}
<dt>Error</dt><dd>{{ .Message }}</dd>
{{ end }}
</dl>
</section>`

// titleTemplate is the heading of the built-in markup: Data.Title, or the
// status text of Data.Status for a Data built without one.
const titleTemplate = `{{ define "title" }}{{ with .Title }}{{ . }}{{ else }}{{ statusText .Status }}{{ end }}{{ end }}`

// WithMode configures the error detail mode.
func WithMode(mode Mode) Option {
	return func(cfg *config) {
		cfg.mode = mode
	}
}

// WithTemplate renders the error page, the failure response of a full
// request, with the template at path, such as a theme's 500.gohtml, instead
// of the built-in page. The template is read from the file system of the
// partial that failed and renders as its child, with its functions, stages,
// includes and @model values; its dot is a Data, which it can declare with
// @dot github.com/donseba/go-partial/ext/errors.Data. Data.Message is
// SafeMessage in ModeSafe and the error in ModeDetailed, or the text of
// WithMessage; Data.Error holds
// the error in both: show it only when Data.Detailed is true. The template
// renders with the includes the partial gives its descendants, so a root
// with SetChildIncludes("partials/*.gohtml") gives an error page the
// partials its pages use. When the template itself fails, the built-in
// page is rendered. Without WithFragmentTemplate, the template answers
// partial requests too.
func WithTemplate(path string) Option {
	return func(cfg *config) {
		if path = strings.TrimSpace(path); path != "" {
			cfg.page = partial.NewID("error", path)
		}
	}
}

// WithFragmentTemplate renders the error fragment, which takes the place of
// a failing part of a page and answers a failing partial request, with the
// template at path, such as a toast, instead of the built-in fragment; see
// WithTemplate. Without WithTemplate, the template renders the error page
// too.
func WithFragmentTemplate(path string) Option {
	return func(cfg *config) {
		if path = strings.TrimSpace(path); path != "" {
			cfg.fragment = partial.NewID("error-fragment", path)
		}
	}
}

// WithMessage sets Data.Message, which the error page, the error fragment
// and the error flash message (see WithFlash) show, to the text that message
// returns, such as a translation of SafeMessage in the request's language.
// message is called for each error render, in both modes, with the data it
// changes: Data.Message is SafeMessage or, when Data.Detailed is true, the
// error. An empty text keeps it.
//
//	errors.Stage(errors.WithMessage(func(ctx *partial.RenderContext, data errors.Data) string {
//		if data.Detailed {
//			return ""
//		}
//
//		return translate(ctx.Request, data.Message)
//	}))
func WithMessage(message func(ctx *partial.RenderContext, data Data) string) Option {
	return func(cfg *config) {
		cfg.message = message
	}
}

// WithLang sets Data.Lang, the language of the error page, to the text
// that lang returns, such as the language of the page that failed, so the
// built-in page, whose text WithMessage may translate, has the right lang
// attribute; a template of the application's own can write it too. lang
// is called for each error render, with the data it changes; an empty text
// leaves Data.Lang empty, which the built-in page writes as en.
//
//	errors.Stage(errors.WithLang(func(ctx *partial.RenderContext, _ errors.Data) string {
//		return localization.Locale(ctx)
//	}))
func WithLang(lang func(ctx *partial.RenderContext, data Data) string) Option {
	return func(cfg *config) {
		cfg.lang = lang
	}
}

// WithTitle sets Data.Title, the heading of the built-in error page and
// fragment, to the text that title returns, such as a translation of the
// status text in the request's language. title is called for each error
// render, with the data it changes: Data.Title is the status text of
// Data.Status (http.StatusText). An empty text keeps it. In ModeDetailed,
// a failed render keeps the heading "Template render error", which is for
// the developer.
//
//	errors.Stage(errors.WithTitle(func(ctx *partial.RenderContext, data errors.Data) string {
//		return translate(ctx.Request, data.Title)
//	}))
func WithTitle(title func(ctx *partial.RenderContext, data Data) string) Option {
	return func(cfg *config) {
		cfg.title = title
	}
}

// WithFlash answers a partial request that fails to render with an
// error flash message instead of the error fragment: added to the flash
// target out of band, with the flash template and target of the tree's
// exp/flash Stage, as the page's own messages appear, and leaving the
// request's target as it is (htmx's HX-Reswap: none). The message is the
// text that message returns, such as a translated sentence; a nil message,
// or an empty text, is Data.Message: SafeMessage in ModeSafe, the error in
// ModeDetailed, or the text of WithMessage. Failing parts of a page, and
// trees without a flash stage, still get the error fragment.
//
//	root.Use(flash.Stage(flash.WithTemplate("flash.gohtml")), errors.Stage(errors.WithFlash(nil)))
//
// Under partial.FailResponse the response keeps status 500: with htmx 4,
// let the page swap the out-of-band parts of 5xx responses with
// hx-status:5xx="swap:none" (connector.Attr.KeepOnServerError); htmx 2
// swaps no error response.
func WithFlash(message func(ctx *partial.RenderContext, data Data) string) Option {
	return func(cfg *config) {
		cfg.flash = true
		cfg.flashMessage = message
	}
}

// Stage returns a render stage that handles error render contexts: an
// error page with status 500, or for partial requests an error fragment with
// status 200, which the client swaps in place. Under partial.FailResponse,
// partial.Write keeps status 500 for the fragment as well. Respond answers
// an error that happened outside a render with the same stage.
func Stage(options ...Option) partial.RenderStage {
	cfg := config{mode: ModeSafe}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	cfg.pairTemplates()

	return partial.RenderStageHooks{
		Package: packagePath,
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

			keepFailureStatus(ctx)

			data := buildData(ctx, cfg)
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

			return renderError(ctx, cfg)
		},
		FinalizeFunc: func(ctx *partial.RenderContext, out template.HTML, renderErr error) (template.HTML, error) {
			if ctx == nil || ctx.Kind != RenderKindError || renderErr == nil {
				return out, renderErr
			}

			data := stateData(ctx, cfg)
			if data.Error == nil {
				return out, renderErr
			}
			return out, stderrors.Join(renderErr, data.Error)
		},
	}
}

// pairTemplates lets the page and the fragment template stand in for each
// other: a stage with only one of them answers with it in both cases.
func (cfg *config) pairTemplates() {
	if cfg.page == nil {
		cfg.page = cfg.fragment
	}

	if cfg.fragment == nil {
		cfg.fragment = cfg.page
	}
}

// keepFailureStatus gives the response of an error of Respond the status
// it was given.
func keepFailureStatus(ctx *partial.RenderContext) {
	if status := ctx.FailureStatus(); status > 0 {
		ctx.Response.Status = status
	}
}

// renderError renders the error of an error render: for a fragment, the
// error flash message with WithFlash; otherwise the fragment, or the page
// for a partial request whose status asks for it, with the application's
// template when it has one and the built-in markup otherwise.
func renderError(ctx *partial.RenderContext, cfg config) (template.HTML, error) {
	data := stateData(ctx, cfg)
	custom := cfg.page
	fragment := false

	if ctx.Name == "fragment" {
		if out, ok := renderFlash(ctx, cfg, data); ok {
			return out, nil
		}

		fragment = !pageForPartialRequest(ctx.FailureStatus())
		if fragment {
			custom = cfg.fragment
		}
	}

	if out, ok := renderCustom(ctx, custom, data); ok {
		return out, nil
	}

	return renderBuiltIn(data, fragment)
}

// pageForPartialRequest reports whether an error of Respond with status
// answers a partial request with the error page rather than the error
// fragment: a 5xx error does, so the interactions client's reloadOnError,
// which shows the HTML of a 5xx answer as the whole page, shows a page.
func pageForPartialRequest(status int) bool {
	return status >= http.StatusInternalServerError
}

// renderBuiltIn renders the built-in error page, or the error fragment.
func renderBuiltIn(data Data, fragment bool) (template.HTML, error) {
	source := pageTemplate
	if fragment {
		source = fragmentTemplate
	}

	tmpl, err := template.New("go-partial-error").Funcs(builtInFuncs).Parse(source + titleTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return template.HTML(buf.String()), nil
}

// builtInFuncs are the functions of the built-in error markup.
var builtInFuncs = template.FuncMap{
	"statusText": http.StatusText,
}

// renderFlash answers a failed partial request with an error flash message,
// with WithFlash; it reports false otherwise, or when the tree has no
// flash stage.
func renderFlash(ctx *partial.RenderContext, cfg config, data Data) (template.HTML, bool) {
	if !cfg.flash || !ctx.FailureResponse() || ctx.Runtime == nil {
		return "", false
	}

	message := ""
	if cfg.flashMessage != nil {
		message = cfg.flashMessage(ctx, data)
	}

	if message == "" {
		message = data.Message
	}

	out, ok := flash.RenderOOB(ctx, flash.Error(message))
	if !ok {
		return "", false
	}

	if ctx.Response != nil {
		if ctx.Response.Headers == nil {
			ctx.Response.Headers = make(map[string]string)
		}

		maps.Copy(ctx.Response.Headers, ctx.Runtime.Connector().ResponseHeaders(connector.Response{Reswap: "none"}))
	}

	return out, true
}

// renderCustom renders the custom error template p with data as a child of
// the partial that failed. It reports false when there is none, or when it
// fails, so the built-in markup answers.
func renderCustom(ctx *partial.RenderContext, p *partial.Partial, data Data) (template.HTML, bool) {
	if p == nil || ctx.Runtime == nil || ctx.Context == nil {
		return "", false
	}

	// A failure inside the custom template is answered with the built-in
	// markup, rather than with the custom template again.
	if _, rendering := FromContext(ctx.Context); rendering {
		return "", false
	}

	ctx.Context = context.WithValue(ctx.Context, renderingKey{}, data)

	out, err := ctx.Runtime.RenderPartial(p.Clone().SetDot(data))
	if err != nil {
		ctx.Emit(partial.Event{
			Kind:    EventTemplateFailed,
			Level:   partial.EventWarn,
			Message: "custom error template failed; the built-in one answers",
			Error:   err,
		})
		return "", false
	}

	return out, true
}

// FromContext returns the Data of the error page or error fragment that a
// custom error template (see WithTemplate and WithFragmentTemplate) renders
// with ctx, and reports whether ctx is the context of such a render: the
// context a template function with a context.Context parameter is called
// with while the template, or a partial it includes, renders. A function
// that writes something of the page that failed, such as its structured
// data, can leave it out of the error page.
//
//	func structuredData(ctx context.Context) template.HTML {
//		if _, failed := errors.FromContext(ctx); failed {
//			return ""
//		}
//		...
//	}
func FromContext(ctx context.Context) (Data, bool) {
	if ctx == nil {
		return Data{}, false
	}

	data, ok := ctx.Value(renderingKey{}).(Data)

	return data, ok
}

func stateData(ctx *partial.RenderContext, cfg config) Data {
	if ctx == nil {
		return BuildData(nil, cfg.mode)
	}
	if ctx.Values != nil {
		if data, ok := ctx.Values.Get(stateKey{}).(Data); ok {
			return data
		}
	}
	if data, ok := ctx.Data.(Data); ok {
		return data
	}
	return buildData(ctx, cfg)
}

// buildData is BuildData with the message of WithMessage, the title of
// WithTitle and the language of WithLang.
func buildData(ctx *partial.RenderContext, cfg config) Data {
	data := BuildData(ctx, cfg.mode)

	if cfg.message != nil {
		if message := cfg.message(ctx, data); message != "" {
			data.Message = message
		}
	}

	if cfg.title != nil {
		if title := cfg.title(ctx, data); title != "" {
			data.Title = title
		}
	}

	if cfg.lang != nil {
		data.Lang = cfg.lang(ctx, data)
	}

	return data
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
		Message:  SafeMessage,
		Status:   http.StatusInternalServerError,
		Detailed: mode == ModeDetailed,
	}
	if data.Detailed {
		data.Message = err.Error()
		data.Location = ExtractTemplateLocation(err)
	}

	if ctx != nil {
		data.Request = ctx.Request
		data.URL = ctx.URL

		// An error of Respond happened outside a render: no partial
		// failed. Which one did is for the developer alone.
		if status := ctx.FailureStatus(); status > 0 {
			data.Status = status
		} else if ctx.Partial != nil && data.Detailed {
			data.PartialID = ctx.Partial.PartialID()
			data.Templates = ctx.Partial.TemplatePaths()
		}
	}

	data.Title = http.StatusText(data.Status)
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
