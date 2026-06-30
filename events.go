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
)

const (
	EventDebug EventLevel = "debug"
	EventInfo  EventLevel = "info"
	EventWarn  EventLevel = "warn"
	EventError EventLevel = "error"
)

const (
	DropNewest DropPolicy = "drop_newest"
	DropOldest DropPolicy = "drop_oldest"
	Block      DropPolicy = "block"
)

const (
	EventRenderStart          = "render.start"
	EventRenderFinish         = "render.finish"
	EventRenderError          = "render.error"
	EventRenderWriteError     = "render.write_error"
	EventRenderOOBError       = "render.oob_error"
	EventTemplateMissing      = "template.missing"
	EventTemplateParseError   = "template.parse_error"
	EventTemplateExecuteError = "template.execute_error"
	EventFuncProtected        = "func.protected"
	EventContentMissingLayout = "content.missing_layout"
	EventTargetMissing        = "target.missing"
	EventContractInvalid      = "contract.invalid"
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

func (ctx *RenderContext) prepareEvent(event Event) Event {
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	if event.Level == "" {
		event.Level = EventInfo
	}
	if event.PartialID == "" && ctx.Partial != nil {
		event.PartialID = ctx.Partial.PartialID()
	}
	if event.ParentID == "" && ctx.Partial != nil {
		event.ParentID = ctx.Partial.ParentID()
	}
	if event.Name == "" {
		event.Name = ctx.Name
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

func emitOnPartial(p *Partial, event Event) {
	if p == nil {
		return
	}
	ctx := newRenderContext(context.Background(), p, p.getRequest(), RenderKindPartial)
	ctx.Emit(event)
}
