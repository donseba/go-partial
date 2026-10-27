package partial

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

type (
	// FailurePolicy decides what a failing part of a render does to the
	// whole render; see Partial.SetFailurePolicy.
	FailurePolicy int

	// renderFailures collects the failures of one render tree for
	// FailResponse. Failures are recorded with every enclosing collector, so
	// an outer render fails when a render it started failed, also when that
	// render's caller ignored the error.
	renderFailures struct {
		mu     sync.Mutex
		parent *renderFailures
		events []Event
	}

	renderFailuresKey struct{}
)

const (
	// FailInline renders a failing part of a page, such as the content
	// child, a {{ partial }} call or a component rendered by a stage, as the
	// error fragment of the registered error stage, or as an error message,
	// in place; the rest of the page renders. This is the default.
	FailInline FailurePolicy = iota

	// FailResponse fails the whole render when any part of it fails, and
	// also when a template, child or content partial it asks for is
	// missing. Render, RenderWithRequest and RenderSelf return the first
	// failure as their error and no HTML. Write writes no part of the page:
	// it writes the failure response of a registered error stage, such as
	// ext/errors, with an error status also for partial requests and
	// without out-of-band regions, and returns a *FailureResponseError; or
	// it writes nothing at all and returns the error, so the caller can
	// answer it. Use it where showing a partly broken page is worse than an
	// error page, such as on a public website.
	FailResponse
)

// SetFailurePolicy sets what a failing part of a render does to the whole:
// FailInline (the default) or FailResponse. Children inherit the policy;
// the policy of the partial passed to Render, RenderWithRequest, RenderSelf
// or Write decides for its whole render.
func (p *Partial) SetFailurePolicy(policy FailurePolicy) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.failurePolicy = uint8(policy) + 1

	return p
}

// getFailurePolicy returns the failure policy of the partial or of its
// nearest ancestor that sets one, FailInline when none does. A partial
// keeps its policy plus one, so zero is no policy of its own.
func (p *Partial) getFailurePolicy() FailurePolicy {
	for current := p; current != nil; {
		current.mu.RLock()
		stored := current.failurePolicy
		parent := current.parent
		current.mu.RUnlock()

		if stored != 0 {
			return FailurePolicy(stored) - 1
		}

		current = parent
	}

	return FailInline
}

// renderTree is the context of a render tree from where it starts, such as
// Write: one context value holds the failures of FailResponse and the lazy
// values the partials of the tree share (see DotFuncOf and ModelFuncOf).
type renderTree struct {
	context.Context

	// failures points to failuresValue when the tree collects failures;
	// lazy is set when it keeps lazy values. Both are nil otherwise.
	failures      *renderFailures
	lazy          *lazyDots
	failuresValue renderFailures
}

// Value answers the keys of the tree's own values, when it has them.
func (t *renderTree) Value(key any) any {
	switch key.(type) {
	case renderFailuresKey:
		if t.failures != nil {
			return t.failures
		}
	case lazyDotsKey:
		if t.lazy != nil {
			return t.lazy
		}
	}

	return t.Context.Value(key)
}

// failingRender prepares ctx for the render of p that starts here: with p's
// failure policy FailResponse it returns ctx with a collector for the
// failures of the render, and the collector; with lazy values on p or an
// ancestor, with a store for them, unless ctx has one. Otherwise it returns
// ctx and nil.
func failingRender(ctx context.Context, r *http.Request, p *Partial) (context.Context, *renderFailures) {
	failing := p.getFailurePolicy() == FailResponse

	lazy := p.hasLazyValues()
	if !failing && !lazy {
		return ctx, nil
	}

	if ctx == nil {
		if r != nil {
			ctx = r.Context()
		} else {
			ctx = defaultRenderContext()
		}
	}

	if lazy {
		if _, kept := ctx.Value(lazyDotsKey{}).(*lazyDots); kept {
			lazy = false
		}
	}

	if !failing && !lazy {
		return ctx, nil
	}

	tree := &renderTree{Context: ctx}
	if failing {
		tree.failuresValue.parent = renderFailuresFrom(ctx)
		tree.failures = &tree.failuresValue
	}

	if lazy {
		tree.lazy = &lazyDots{}
	}

	return tree, tree.failures
}

func renderFailuresFrom(ctx context.Context) *renderFailures {
	if ctx == nil {
		return nil
	}

	failures, _ := ctx.Value(renderFailuresKey{}).(*renderFailures)

	return failures
}

// isFailureEvent reports whether event tells that a part of a render
// failed or is missing.
func isFailureEvent(event Event) bool {
	switch event.Kind {
	case EventRenderError, EventRenderOOBError, EventTemplateMissing, EventTemplateParseError,
		EventTemplateExecuteError, EventContractInvalid, EventContentMissing:
		return true
	default:
		return false
	}
}

func (f *renderFailures) record(event Event) {
	for current := f; current != nil; current = current.parent {
		current.mu.Lock()
		current.events = append(current.events, event)
		current.mu.Unlock()
	}
}

// err is the failure that caused the others, or nil: the first recorded
// error, as the innermost render reports its failure first and the renders
// around it report theirs after.
func (f *renderFailures) err() error {
	if f == nil {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, event := range f.events {
		if event.Error != nil {
			return fmt.Errorf("render %s: %w", event.PartialID, event.Error)
		}
	}

	if len(f.events) == 0 {
		return nil
	}

	event := f.events[0]

	return fmt.Errorf("render %s: %s %v", event.PartialID, event.Message, event.Fields)
}
