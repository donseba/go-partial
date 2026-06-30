package partial

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type (
	// EventLevel describes the severity of a diagnostic event.
	EventLevel string

	// Event describes something observable that happened during rendering.
	Event struct {
		Time         time.Time
		Kind         string
		Level        EventLevel
		Message      string
		Error        error
		Fields       map[string]any
		TraceID      string
		SpanID       string
		ParentSpanID string
		PartialID    string
		ParentID     string
		Name         string
	}

	// EventSink receives diagnostic events. Sinks should be defensive and cheap.
	EventSink interface {
		Emit(*RenderContext, Event)
	}

	// EventSinkFunc adapts a function to EventSink.
	EventSinkFunc func(*RenderContext, Event)

	// FanoutEventSink forwards events to multiple sinks.
	FanoutEventSink []EventSink

	// DropPolicy decides what AsyncEvents does when its buffer is full.
	DropPolicy string

	// EventsConfig configures AsyncEvents.
	EventsConfig struct {
		Buffer     int
		DropPolicy DropPolicy
		Workers    int
	}

	// AsyncEvents is a non-blocking event dispatcher backed by a bounded queue.
	AsyncEvents struct {
		sinks  []EventSink
		ch     chan eventEnvelope
		done   chan struct{}
		once   sync.Once
		closed atomic.Bool
		drops  atomic.Uint64
		wg     sync.WaitGroup
		policy DropPolicy
	}

	eventEnvelope struct {
		ctx   *RenderContext
		event Event
	}

	eventSinkContextKey struct{}
)

const (
	// EventDebug is useful for local tracing and lifecycle views.
	EventDebug EventLevel = "debug"
	// EventInfo describes normal diagnostic breadcrumbs.
	EventInfo EventLevel = "info"
	// EventWarn describes recoverable problems or ignored configuration.
	EventWarn EventLevel = "warn"
	// EventError describes render failures or other errors.
	EventError EventLevel = "error"
)

const (
	// DropNewest drops the incoming event when the queue is full.
	DropNewest DropPolicy = "drop_newest"
	// DropOldest drops one queued event to make room for the incoming event.
	DropOldest DropPolicy = "drop_oldest"
	// Block waits for queue space or Close. Use only when event loss is worse than render latency.
	Block DropPolicy = "block"
)

const (
	// EventRenderStart is emitted before a render chain runs.
	EventRenderStart = "render.start"
	// EventRenderFinish is emitted after a render chain succeeds.
	EventRenderFinish = "render.finish"
	// EventRenderError is emitted after a render chain fails.
	EventRenderError = "render.error"
	// EventRenderWriteError is emitted when writing a render response fails.
	EventRenderWriteError = "render.write_error"
	// EventRenderOOBError is emitted when an out-of-band child render fails.
	EventRenderOOBError = "render.oob_error"
	// EventTemplateMissing is emitted when a helper references a missing template.
	EventTemplateMissing = "template.missing"
	// EventTemplateParseError is emitted when parsing or loading templates fails.
	EventTemplateParseError = "template.parse_error"
	// EventTemplateExecuteError is emitted when template execution fails.
	EventTemplateExecuteError = "template.execute_error"
	// EventFuncProtected is emitted when a user function tries to overwrite a protected helper.
	EventFuncProtected = "func.protected"
	// EventContentMissingLayout is emitted when content is called outside a layout wrapper.
	EventContentMissingLayout = "content.missing_layout"
	// EventTargetMissing is emitted when a requested target cannot be resolved.
	EventTargetMissing = "target.missing"
	// EventContractInvalid is emitted when contract data or helper arguments are invalid.
	EventContractInvalid = "contract.invalid"
)

func timeNow() time.Time {
	return time.Now()
}

// Emit sends event to the wrapped function.
func (f EventSinkFunc) Emit(ctx *RenderContext, event Event) {
	if f != nil {
		f(ctx, event)
	}
}

// FanoutEvents returns a sink that forwards events to each non-nil sink.
func FanoutEvents(sinks ...EventSink) FanoutEventSink {
	return FanoutEventSink(sinks)
}

// Emit sends event to each configured sink.
func (sinks FanoutEventSink) Emit(ctx *RenderContext, event Event) {
	for _, sink := range sinks {
		emitSafely(sink, ctx, event)
	}
}

// WithEventSink attaches a request-scoped event sink to ctx.
//
// Request-scoped sinks are fanned out with service or partial sinks for renders
// started with this context. If the sink owns goroutines, the caller still owns
// closing it, usually with defer in middleware.
//
// If ctx already has a request-scoped sink, the new sink is appended with fanout.
func WithEventSink(ctx context.Context, sink EventSink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sink == nil {
		return ctx
	}
	sink = mergeEventSinks(EventSinkFromContext(ctx), sink)
	return context.WithValue(ctx, eventSinkContextKey{}, sink)
}

// EventSinkFromContext returns the request-scoped event sink attached to ctx.
func EventSinkFromContext(ctx context.Context) EventSink {
	if ctx == nil {
		return nil
	}
	sink, _ := ctx.Value(eventSinkContextKey{}).(EventSink)
	return sink
}

// NewAsyncEvents returns a non-blocking dispatcher for diagnostic events.
func NewAsyncEvents(cfg EventsConfig, sinks ...EventSink) *AsyncEvents {
	if cfg.Buffer < 0 {
		cfg.Buffer = 0
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.DropPolicy == "" {
		cfg.DropPolicy = DropNewest
	}
	dispatcher := &AsyncEvents{
		sinks:  append([]EventSink(nil), sinks...),
		ch:     make(chan eventEnvelope, cfg.Buffer),
		done:   make(chan struct{}),
		policy: cfg.DropPolicy,
	}
	for range cfg.Workers {
		dispatcher.wg.Add(1)
		go dispatcher.work()
	}
	return dispatcher
}

// Emit enqueues event without blocking unless configured with Block.
func (events *AsyncEvents) Emit(ctx *RenderContext, event Event) {
	if events == nil || events.closed.Load() {
		return
	}
	defer func() {
		_ = recover()
	}()
	envelope := eventEnvelope{ctx: ctx, event: event}
	switch events.policy {
	case Block:
		select {
		case events.ch <- envelope:
		case <-events.done:
		}
	case DropOldest:
		select {
		case events.ch <- envelope:
			return
		default:
		}
		select {
		case <-events.ch:
			events.drops.Add(1)
		default:
		}
		select {
		case events.ch <- envelope:
		default:
			events.drops.Add(1)
		}
	default:
		select {
		case events.ch <- envelope:
		default:
			events.drops.Add(1)
		}
	}
}

// Dropped returns the number of events dropped by this dispatcher.
func (events *AsyncEvents) Dropped() uint64 {
	if events == nil {
		return 0
	}
	return events.drops.Load()
}

// Close stops workers after delivering queued events, or returns when ctx ends.
func (events *AsyncEvents) Close(ctx context.Context) error {
	if events == nil {
		return nil
	}
	events.once.Do(func() {
		events.closed.Store(true)
		close(events.done)
		close(events.ch)
	})
	finished := make(chan struct{})
	go func() {
		events.wg.Wait()
		close(finished)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Emit sends a diagnostic event through the active render context.
func (ctx *RenderContext) Emit(event Event) {
	if ctx == nil || ctx.Events == nil {
		return
	}
	event = ctx.prepareEvent(event)
	emitSafely(ctx.Events, ctx, event)
}

// EmitForPartial sends an event for a specific partial while preserving the
// active render context, including request-scoped event sinks.
func (ctx *RenderContext) EmitForPartial(p *Partial, event Event) {
	if ctx == nil {
		return
	}
	if event.PartialID == "" && p != nil {
		event.PartialID = p.PartialID()
	}
	if event.ParentID == "" && p != nil {
		event.ParentID = p.ParentID()
	}
	ctx.Emit(event)
}

func (ctx *RenderContext) prepareEvent(event Event) Event {
	return preparePartialEvent(ctx.Partial, event).withName(ctx.Name)
}

func preparePartialEvent(p *Partial, event Event) Event {
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	if event.Level == "" {
		event.Level = EventInfo
	}
	if event.PartialID == "" && p != nil {
		event.PartialID = p.PartialID()
	}
	if event.ParentID == "" && p != nil {
		event.ParentID = p.ParentID()
	}
	return event
}

func (event Event) withName(name string) Event {
	if event.Name == "" {
		event.Name = name
	}
	return event
}

func (events *AsyncEvents) work() {
	defer events.wg.Done()
	for envelope := range events.ch {
		for _, sink := range events.sinks {
			emitSafely(sink, envelope.ctx, envelope.event)
		}
	}
}

func emitSafely(sink EventSink, ctx *RenderContext, event Event) {
	if sink == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	sink.Emit(ctx, event)
}

func mergeEventSinks(a, b EventSink) EventSink {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return FanoutEvents(a, b)
	}
}
