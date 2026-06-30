// Package metrics provides an experimental renderer for passive render metrics.
package metrics

import (
	"context"
	"html/template"
	"net/http"
	"time"

	partial "github.com/donseba/go-partial"
)

type (
	// Record describes one completed render lifecycle.
	Record struct {
		Kind       partial.RenderKind
		Name       string
		RequestID  string
		PartialID  string
		ParentID   string
		PartialTag string
		SlotName   string
		Templates  []string
		OOB        bool
		Method     string
		Path       string
		Size       int
		Rendered   bool
		StartedAt  time.Time
		Duration   time.Duration
		Error      error
		Tags       map[string]string
	}

	// Sink receives render metric records.
	Sink interface {
		Record(Record)
	}

	// SinkFunc adapts a function to a metrics sink.
	SinkFunc func(Record)

	// FanoutSink sends each record to multiple sinks.
	FanoutSink []Sink

	config struct {
		sink     Sink
		now      func() time.Time
		tags     map[string]string
		slotName func(*partial.Partial) string
	}

	// Option configures the metrics renderer.
	Option func(*config)

	partialTagKey struct{}
	requestIDKey  struct{}
	stateKey      struct{}

	state struct {
		startedAt time.Time
		rendered  bool
	}
)

const (
	// HeaderRequestID is the conventional HTTP header used as a metrics request ID fallback.
	HeaderRequestID = "X-Request-ID"
)

// Record sends a metrics record to the wrapped function.
func (f SinkFunc) Record(record Record) {
	if f != nil {
		f(record)
	}
}

// Fanout returns a sink that forwards each record to every non-nil sink.
func Fanout(sinks ...Sink) FanoutSink {
	return FanoutSink(sinks)
}

// Record sends record to every configured sink.
func (sinks FanoutSink) Record(record Record) {
	for _, sink := range sinks {
		if sink != nil {
			sink.Record(record)
		}
	}
}

// WithRequestID stores a request ID for metrics records created from this context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFromContext returns the metrics request ID stored in ctx.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// WithPartialTag labels a partial in emitted metrics records.
func WithPartialTag(p *partial.Partial, tag string) *partial.Partial {
	if p == nil || tag == "" {
		return p
	}
	return p.SetExtension(partialTagKey{}, tag)
}

// PartialTag returns the metrics label configured for a partial.
func PartialTag(p *partial.Partial) string {
	if p == nil {
		return ""
	}
	tag, _ := p.Extension(partialTagKey{})
	value, _ := tag.(string)
	return value
}

// WithTag adds a static tag to each emitted record.
func WithTag(key, value string) Option {
	return func(cfg *config) {
		if key == "" {
			return
		}
		if cfg.tags == nil {
			cfg.tags = make(map[string]string)
		}
		cfg.tags[key] = value
	}
}

// WithClock configures the clock used by the renderer.
func WithClock(now func() time.Time) Option {
	return func(cfg *config) {
		if now != nil {
			cfg.now = now
		}
	}
}

// WithSlotName configures how metrics discovers a partial's slot name.
func WithSlotName(slotName func(*partial.Partial) string) Option {
	return func(cfg *config) {
		cfg.slotName = slotName
	}
}

// Renderer returns a passive renderer that records render timing and output size.
func Renderer(sink Sink, options ...Option) partial.Renderer {
	cfg := config{
		sink: sink,
		now:  time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return partial.RendererHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			if ctx == nil {
				return ctx, nil
			}
			ensureValues(ctx).Set(stateKey{}, &state{startedAt: cfg.now()})
			return ctx, nil
		},
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if current := getState(ctx); current != nil {
				current.rendered = true
			}
			return next(ctx)
		},
		FinalizeFunc: func(ctx *partial.RenderContext, out template.HTML, renderErr error) (template.HTML, error) {
			if cfg.sink != nil {
				cfg.sink.Record(buildRecord(ctx, out, renderErr, cfg))
			}
			return out, renderErr
		},
	}
}

func buildRecord(ctx *partial.RenderContext, out template.HTML, renderErr error, cfg config) Record {
	finishedAt := cfg.now()
	startedAt := finishedAt
	if current := getState(ctx); current != nil && !current.startedAt.IsZero() {
		startedAt = current.startedAt
	}
	rendered := false
	if current := getState(ctx); current != nil {
		rendered = current.rendered
	}

	record := Record{
		StartedAt: startedAt,
		Duration:  finishedAt.Sub(startedAt),
		Size:      len([]byte(out)),
		Rendered:  rendered,
		Error:     renderErr,
		Tags:      cloneTags(cfg.tags),
	}

	if ctx == nil {
		return record
	}

	record.Kind = ctx.Kind
	record.Name = ctx.Name
	record.RequestID = requestID(ctx)
	if ctx.Partial != nil {
		record.PartialID = ctx.Partial.PartialID()
		record.ParentID = ctx.Partial.ParentID()
		record.PartialTag = PartialTag(ctx.Partial)
		if cfg.slotName != nil {
			record.SlotName = cfg.slotName(ctx.Partial)
		}
		record.Templates = ctx.Partial.TemplatePaths()
		record.OOB = ctx.Partial.IsOOB()
	}
	if ctx.Request != nil {
		record.Method = ctx.Request.Method
		record.Path = requestPath(ctx.Request)
	}
	return record
}

func ensureValues(ctx *partial.RenderContext) partial.RenderValues {
	if ctx.Values == nil {
		ctx.Values = make(partial.RenderValues)
	}
	return ctx.Values
}

func getState(ctx *partial.RenderContext) *state {
	if ctx == nil || ctx.Values == nil {
		return nil
	}
	current, _ := ctx.Values.Get(stateKey{}).(*state)
	return current
}

func requestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}

func requestID(ctx *partial.RenderContext) string {
	if ctx == nil {
		return ""
	}
	if id := RequestIDFromContext(ctx.Context); id != "" {
		return id
	}
	if ctx.Request == nil {
		return ""
	}
	return ctx.Request.Header.Get(HeaderRequestID)
}

func cloneTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		out[key] = value
	}
	return out
}
