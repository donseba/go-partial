// Package localization provides experimental locale helpers for templates.
package localization

import (
	"context"
	"html/template"
	"reflect"

	partial "github.com/donseba/go-partial"
)

var (
	// Default is the fallback Localizer used when no localizer is configured.
	Default = Localizer(defaultLocalizer{locale: "en_US"})
)

var localizerContextKey = contextKey{}

type contextKey struct{}

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[contextKey]().PkgPath()

// Localizer provides the active locale for a render.
type Localizer interface {
	GetLocale() string
}

type defaultLocalizer struct {
	locale string
}

// FuncMap returns placeholders for localization template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"locale":    Locale,
		"localizer": LocalizerValue,
	}
}

// LocalizerValue returns the configured localizer for a render context.
//
// go-doc:sig func() github.com/donseba/go-partial/exp/localization.Localizer
func LocalizerValue(ctx ...*partial.RenderContext) Localizer {
	if len(ctx) == 0 || ctx[0] == nil {
		return Default
	}
	return FromContext(ctx[0].Context)
}

// Locale returns the configured locale for a render context.
//
// go-doc:sig func() string
func Locale(ctx ...*partial.RenderContext) string {
	return LocalizerValue(ctx...).GetLocale()
}

// Stage gives templates the locale and localizer helpers. They are made
// only for the templates that call them.
func Stage() partial.RenderStage {
	return partial.RenderStageHooks{
		Package:     packagePath,
		FuncNames:   []string{"localizer", "locale"},
		ResolveFunc: resolveFunc,
	}
}

// resolveFunc gives a template that calls them the localization helpers of
// its render.
func resolveFunc(ctx *partial.RenderContext, name string) (any, bool) {
	switch name {
	case "localizer":
		return func() Localizer {
			return LocalizerValue(ctx)
		}, true
	case "locale":
		return func() string {
			return Locale(ctx)
		}, true
	default:
		return nil, false
	}
}

// New returns a Localizer for the locale code, such as "nl" or "en_US", for
// renders and templates that have no localizer of their own.
func New(code string) Localizer {
	return defaultLocalizer{locale: code}
}

// Funcs returns the locale and localizer helpers bound to l, for templates
// go-partial does not execute, such as the text part of an e-mail written
// with text/template: they give those templates what the Stage gives the
// templates go-partial renders.
//
//	text := texttemplate.New("mail").Funcs(texttemplate.FuncMap(localization.Funcs(localization.New("nl"))))
func Funcs(l Localizer) template.FuncMap {
	if l == nil {
		l = Default
	}

	return template.FuncMap{
		// go-doc:sig func() github.com/donseba/go-partial/exp/localization.Localizer
		"localizer": func() Localizer {
			return l
		},
		// go-doc:sig func() string
		"locale": func() string {
			return l.GetLocale()
		},
	}
}

// WithLocalizer stores a Localizer on a context.
func WithLocalizer(ctx context.Context, localizer Localizer) context.Context {
	return context.WithValue(ctx, localizerContextKey, localizer)
}

// FromContext returns the configured Localizer or Default.
func FromContext(ctx context.Context) Localizer {
	if ctx == nil {
		return Default
	}
	if loc, ok := ctx.Value(localizerContextKey).(Localizer); ok {
		return loc
	}
	return Default
}

func (d defaultLocalizer) GetLocale() string {
	return d.locale
}
