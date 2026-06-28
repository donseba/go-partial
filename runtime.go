package partial

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"github.com/donseba/go-partial/connector"
)

// Runtime is the per-render handle passed explicitly to helpers that need
// request-aware behavior. It keeps application data typed through SetDot while
// still giving advanced helpers access to the active request, connector, and
// partial render path.
type Runtime struct {
	partial *Partial
	state   *RenderContext
}

func newRuntime(p *Partial, state *RenderContext) *Runtime {
	return &Runtime{partial: p, state: state}
}

func (r *Runtime) Context() context.Context {
	if r == nil || r.state == nil {
		return context.Background()
	}
	return r.state.Context
}

func (r *Runtime) Request() *http.Request {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Request
}

func (r *Runtime) URL() *url.URL {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.URL
}

func (r *Runtime) Localizer() Localizer {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Loc
}

func (r *Runtime) Locale() string {
	if r == nil || r.state == nil {
		return ""
	}
	if r.state.Locale != "" {
		return r.state.Locale
	}
	if r.state.Loc != nil {
		return r.state.Loc.GetLocale()
	}
	return ""
}

func (r *Runtime) Csrf() CsrfToken {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Csrf
}

func (r *Runtime) BasePath() string {
	if r == nil || r.state == nil {
		return ""
	}
	return r.state.BasePath
}

func (r *Runtime) Connector() connector.Connector {
	if r == nil || r.partial == nil {
		return nil
	}
	return r.partial.getConnector()
}

// Partial renders a template path through the current partial tree.
func (r *Runtime) Partial(path string, args ...any) template.HTML {
	if r == nil || r.partial == nil || r.state == nil {
		return escapedRuntimeError(fmt.Errorf("go-partial runtime partial renderer is not configured"))
	}
	return partialFunc(r.partial, r.state)(path, args...)
}

// Debug renders a diagnostic value using the active partial debug renderer.
func (r *Runtime) Debug(value any) template.HTML {
	if r == nil || r.partial == nil || r.state == nil {
		return template.HTML(template.HTMLEscapeString(fmt.Sprintf("%#v", value)))
	}
	return debugFunc(r.partial, r.state)(value)
}

func escapedRuntimeError(err error) template.HTML {
	return template.HTML(template.HTMLEscapeString(err.Error()))
}
