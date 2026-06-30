// Package csrf provides experimental CSRF token helpers for templates.
package csrf

import (
	"context"
	"fmt"
	"html/template"
	"time"

	partial "github.com/donseba/go-partial"
)

// DefaultTokenKey is the default form/header name used by fallback tokens.
const DefaultTokenKey = "X-CSRF-Token"

var tokenContextKey = contextKey{}

type contextKey struct{}

// Token describes the CSRF token exposed to templates.
type Token interface {
	Token(ctx context.Context) string
	Key() string
}

type defaultToken struct {
	token string
	key   string
}

// FuncMap returns placeholders for the csrf template helper.
//
// go-doc:funcmap
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"csrf": CSRF,
	}
}

// CSRF returns the configured token for a render context.
//
// go-doc:sig func() github.com/donseba/go-partial/exp/csrf.Token
func CSRF(ctx ...*partial.RenderContext) Token {
	if len(ctx) == 0 || ctx[0] == nil {
		return FromContext(nil)
	}
	return FromContext(ctx[0].Context)
}

// Renderer installs the csrf template helper from the render context.
func Renderer() partial.Renderer {
	return partial.RendererHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			ctx.SetFunc("csrf", func() Token { return CSRF(ctx) })
			return ctx, nil
		},
	}
}

// WithToken stores a Token on a context.
func WithToken(ctx context.Context, token Token) context.Context {
	return context.WithValue(ctx, tokenContextKey, token)
}

// WithTokenString stores a raw token string on a context.
func WithTokenString(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenContextKey, token)
}

// FromContext returns the configured Token or an invalid fallback token.
func FromContext(ctx context.Context) Token {
	if ctx != nil {
		if token, ok := ctx.Value(tokenContextKey).(Token); ok {
			return token
		}
	}
	return &defaultToken{
		token: fmt.Sprintf("invalid-token-%d", time.Now().UnixNano()),
		key:   DefaultTokenKey,
	}
}

func (d *defaultToken) Token(ctx context.Context) string {
	if ctx != nil {
		if token, ok := ctx.Value(tokenContextKey).(string); ok {
			return token
		}
	}
	return d.token
}

func (d *defaultToken) Key() string {
	return d.key
}
