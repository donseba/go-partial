package partial

import (
	"context"
	"net/http"
	"strings"

	"github.com/donseba/go-partial/connector"
)

// TargetFallback answers partial requests for a target the partial tree
// cannot resolve, such as a link or form in another page's layout that
// targets an element this page does not have. Instead of failing, the
// request is answered as one for the fallback partial, usually the page's
// content, and the response tells the client to swap it where that partial
// lives, as a navigation. See Partial.SetTargetFallback.
type TargetFallback struct {
	// ID is the partial rendered instead: the partial that sets the
	// fallback, or a child registered on its tree, such as the content
	// child.
	ID string
	// Retarget is the selector the client swaps the response into; "#" + ID
	// when empty.
	Retarget string
	// Swap is how the client swaps it; outerHTML when nil.
	Swap *connector.Swap
	// PushURL pushes the URL of a GET request to the browser history, so
	// the address follows the navigation.
	PushURL bool
}

// SetTargetFallback answers partial requests for unknown targets with the
// fallback partial rather than an error; see TargetFallback. Children
// inherit the fallback. The response instructions are serialized by the
// connector, such as HX-Retarget, HX-Reswap and HX-Push-Url for htmx, and
// written by Write; RenderWithRequest returns the fallback's HTML.
//
//	root.SetContent(content).
//	    SetTargetFallback(partial.TargetFallback{ID: "content", PushURL: true})
func (p *Partial) SetTargetFallback(fallback TargetFallback) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	fallback.ID = strings.TrimSpace(fallback.ID)
	if fallback.ID == "" {
		p.targetFallback = nil
		return p
	}

	if fallback.Swap != nil {
		swap := *fallback.Swap
		fallback.Swap = &swap
	}

	p.targetFallback = &fallback

	return p
}

func (p *Partial) getTargetFallback() *TargetFallback {
	for current := p; current != nil; {
		current.mu.RLock()
		fallback := current.targetFallback
		parent := current.parent
		current.mu.RUnlock()

		if fallback != nil {
			return fallback
		}

		current = parent
	}

	return nil
}

// renderTargetFallback answers a request for the unknown target with the
// partial's target fallback, if it has one that the tree contains.
func renderTargetFallback(ctx context.Context, r *http.Request, p *Partial, target string) (renderResult, bool) {
	fallback := p.getTargetFallback()
	if fallback == nil || r == nil {
		return renderResult{}, false
	}

	answer := p
	if fallback.ID != p.id {
		answer = p.recursiveChildLookup(fallback.ID, make(map[string]bool))
	}

	if answer == nil {
		return renderResult{}, false
	}

	p.emitWithContext(ctx, r, Event{
		Kind:    EventTargetFallback,
		Level:   EventInfo,
		Message: "requested target answered by the target fallback",
		Fields:  map[string]any{"target": target, "fallback": fallback.ID},
	})

	// Render as if the client had asked for the fallback, so templates and
	// stages that read the target see the partial they render.
	conn := p.getConnectorOrDefault()

	retargeted := r.Clone(r.Context())
	if header := conn.GetTargetHeader(); header != "" {
		retargeted.Header.Set(header, fallback.ID)
	}

	result := renderTargetSelfResult(ctx, retargeted, answer)
	if result.Err != nil {
		return result, true
	}

	response := connector.NewResponseBuilder(nil)
	if fallback.Retarget != "" {
		response.Retarget(fallback.Retarget)
	} else {
		response.Retarget("#" + fallback.ID)
	}

	if fallback.Swap != nil {
		response.ReswapWith(fallback.Swap)
	} else {
		response.ReswapWith(connector.NewSwap().Style(connector.SwapOuterHTML))
	}

	if fallback.PushURL && r.Method == http.MethodGet && r.URL != nil {
		response.PushURL(r.URL.RequestURI())
	}

	if result.Response == nil {
		result.Response = &RenderResponse{}
	}

	if result.Response.Headers == nil {
		result.Response.Headers = make(map[string]string)
	}

	// Headers the render set itself take precedence.
	for key, value := range conn.ResponseHeaders(response.Value()) {
		if _, set := result.Response.Headers[key]; !set {
			result.Response.Headers[key] = value
		}
	}

	return result, true
}
