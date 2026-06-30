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

// Context returns the context for the active render.
func (r *Runtime) Context() context.Context {
	if r == nil || r.state == nil {
		return defaultRenderContext()
	}
	return r.state.Context
}

// Request returns the request for the active render, if any.
func (r *Runtime) Request() *http.Request {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Request
}

// URL returns the request URL for the active render, if any.
func (r *Runtime) URL() *url.URL {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.URL
}

// BasePath returns the active render base path.
func (r *Runtime) BasePath() string {
	if r == nil || r.state == nil {
		return ""
	}
	return r.state.BasePath
}

// RenderContext returns the active render context.
func (r *Runtime) RenderContext() *RenderContext {
	if r == nil {
		return nil
	}
	return r.state
}

// Value returns a value from the active render context.
func (r *Runtime) Value(key any) any {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Values.Get(key)
}

// SetValue stores a value in the active render context.
func (r *Runtime) SetValue(key any, value any) {
	if r == nil || r.state == nil {
		return
	}
	if r.state.Values == nil {
		r.state.Values = make(RenderValues)
	}
	r.state.Values.Set(key, value)
}

// Connector returns the connector for the active partial.
func (r *Runtime) Connector() connector.Connector {
	if r == nil || r.partial == nil {
		return connector.NewPartial(nil)
	}
	return r.partial.getConnectorOrDefault()
}

// Partial renders a template path through the current partial tree.
func (r *Runtime) Partial(path string, args ...any) template.HTML {
	if r == nil || r.partial == nil || r.state == nil {
		return escapedRuntimeError(fmt.Errorf("go-partial runtime partial render stage is not configured"))
	}
	return partialFunc(r.partial, r.state)(path, args...)
}

// RenderPartial renders a child partial through the current render stage chain.
func (r *Runtime) RenderPartial(p *Partial) (template.HTML, error) {
	child, err := r.preparePartial(p)
	if err != nil {
		return "", err
	}
	result := child.renderSelfResult(r.state.Context, r.state.Request)
	return result.HTML, result.Err
}

// RenderPartialWithFallback renders a partial through the current render stage
// chain and returns the configured error fragment when rendering fails.
func (r *Runtime) RenderPartialWithFallback(p *Partial) (template.HTML, error) {
	child, err := r.preparePartial(p)
	if err != nil {
		return "", err
	}

	result := child.renderSelfResult(r.state.Context, r.state.Request)
	if result.Err == nil {
		return result.HTML, nil
	}

	fallback, fallbackErr := child.renderErrorFragment(r.state.Context, r.state.Request, result.Err)
	if fallbackErr != nil {
		return "", fallbackErr
	}
	return fallback, nil
}

// RenderWith renders a non-template task through the active render stage chain.
func (r *Runtime) RenderWith(kind RenderKind, name string, data any, terminal RenderNext) (template.HTML, error) {
	if r == nil || r.partial == nil || r.state == nil {
		return "", fmt.Errorf("go-partial runtime render stage is not configured")
	}
	if terminal == nil {
		return "", fmt.Errorf("terminal render stage is not configured")
	}

	state := newRenderContext(r.state.Context, r.partial, r.state.Request, kind)
	state.Name = name
	state.Data = data
	state.Values = r.state.Values.Clone()
	state.Response = r.state.Response
	state.Runtime = newRuntime(r.partial, state)
	return renderWithChain(state, r.partial.getRenderStages(), terminal)
}

func (r *Runtime) preparePartial(p *Partial) (*Partial, error) {
	if r == nil || r.partial == nil || r.state == nil {
		return nil, fmt.Errorf("go-partial runtime partial render stage is not configured")
	}
	if p == nil {
		return nil, fmt.Errorf("partial is not initialized")
	}
	child := p.clone()
	child.parent = r.partial
	if !child.stagesInherited {
		child.stages = append(append([]RenderStage(nil), r.partial.getRenderStages()...), child.stages...)
		child.stagesInherited = true
	}
	return child, nil
}

func escapedRuntimeError(err error) template.HTML {
	return template.HTML(template.HTMLEscapeString(err.Error()))
}
