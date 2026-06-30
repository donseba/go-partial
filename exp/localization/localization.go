// Package localization provides experimental locale helpers for templates.
package localization

import (
	"context"
	"html/template"

	partial "github.com/donseba/go-partial"
)

var (
	// Default is the fallback Localizer used when no localizer is configured.
	Default = Localizer(defaultLocalizer{locale: "en_US"})
)

var localizerContextKey = contextKey{}

type contextKey struct{}

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

// Renderer installs locale and localizer template helpers.
func Renderer() partial.Renderer {
	return partial.RendererHooks{
		PreflightFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			ctx.SetFunc("localizer", func() Localizer { return LocalizerValue(ctx) })
			ctx.SetFunc("locale", func() string { return Locale(ctx) })
			return ctx, nil
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
