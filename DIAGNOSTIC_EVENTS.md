# Diagnostic Events

This note captures the event model used by go-partial. For application-facing
setup examples, see [OBSERVABILITY.md](OBSERVABILITY.md).

## Goal

Core should not own logging, metrics, tracing, or queues directly. Instead,
core and extensions can emit small structured diagnostic events, and optional
consumers can decide what to do with them.

Examples of consumers:

- `ext/logger` writes events to `log/slog`.
- `exp/metrics` can optionally record event breadcrumbs, although the showcase
  keeps render metrics and diagnostic logs separate.
- `exp/otel` adds events to the active OpenTelemetry span.
- A showcase or devtools page streams events over SSE.
- Application code forwards events to Sentry, Honeycomb, stdout, JSONL, or a
  custom channel.

## Event Meaning

An event should describe what happened, not where it should go.

Prefer:

```text
render.start
render.finish
target.resolve
slot.render
template.missing
```

Avoid destination-style event names:

```text
log
metric
```

Logger and metrics are consumers. They should listen to event facts and decide
whether those facts are useful.

## Minimal Shape

The core contract:

```go
type EventLevel string

const (
	EventDebug EventLevel = "debug"
	EventInfo  EventLevel = "info"
	EventWarn  EventLevel = "warn"
	EventError EventLevel = "error"
)

type Event struct {
	Time         time.Time
	TraceID      string
	SpanID       string
	ParentSpanID string

	Kind    string
	Level   EventLevel
	Message string
	Error   error

	PartialID string
	ParentID  string
	Name      string

	Fields map[string]any
}

type EventSink interface {
	Emit(*RenderContext, Event)
}

type EventSinkFunc func(*RenderContext, Event)
```

Core also exposes a small fanout helper:

```go
func FanoutEvents(sinks ...EventSink) EventSink
```

## Emission

Events should be emitted through the active render context when available:

```go
ctx.Emit(partial.Event{
	Kind:    partial.EventRenderStart,
	Level:   partial.EventDebug,
	Message: "render started",
	Fields: map[string]any{
		"template": "templates/page.gohtml",
	},
})
```

Implemented attachment points:

- `RenderContext.Emit(event)` is convenient for Stages.
- `Partial.SetEvents` attaches sinks to a reusable partial tree.
- `partial.WithEventSink(ctx, sink)` attaches request-scoped sinks. Repeated
  calls append with fanout.

## Initial Vocabulary

Core should start with a small event vocabulary for things it already logs or
already knows.

Suggested core events:

```text
render.start
render.finish
render.error
render.write_error
render.oob_error
template.missing
template.parse_error
func.protected
content.missing
target.missing
```

Extensions should namespace their events:

```text
ext.debug.render
ext.errors.render
exp.selection.resolve
exp.target.resolve
exp.actions.resolve
exp.slots.render
exp.metrics.record
exp.sse.patch
```

## Logger

Logging should become an extension, not a core behavior.

`ext/logger` can expose a sink:

```go
logger.Sink(slog.Default())
```

It can map levels to `slog`:

```text
debug -> slog.Debug
info  -> slog.Info
warn  -> slog.Warn
error -> slog.Error
```

The logger sink should not decide render behavior. It only observes.

## Metrics

`exp/metrics` can continue using the Render stage lifecycle for precise timing and
HTML size. Diagnostic events can add semantic context:

- which target resolver was attempted
- which selection key was chosen
- which slot rendered
- whether an OOB child failed
- why a fallback/error fragment was emitted

Metrics should not require every event. It should consume the events it
understands and ignore the rest.

## Trace And Debug

Trace consumers can use request context, `TraceID`, `SpanID`, and
`ParentSpanID` to build or enrich a request tree:

```text
GET /metrics
  render tree
    slot header
      render header
    render content
      exp.metrics.record
```

Render stage lifecycle events are best for call-stack timing. Extension events are
best for semantic breadcrumbs.

`exp/otel` keeps OpenTelemetry policy outside core. It records go-partial events
on the active span but does not create spans, exporters, or providers.

## Async Dispatch

Core exposes `partial.NewAsyncEvents` as the recommended dispatcher.

Async dispatch introduces policy:

- buffering
- backpressure
- goroutines
- shutdown
- dropped-event behavior
- ordering guarantees

`NewAsyncEvents` keeps rendering from waiting on slow consumers by using a
bounded queue. The default policy is `DropNewest`; applications can choose
`DropOldest` or explicit `Block` if preserving every event matters more than
render latency.

## Rules Of Thumb

- Events observe. They do not steer rendering.
- Event kinds describe facts, not destinations.
- Core emits only low-volume diagnostic events.
- Extensions may emit namespaced events.
- Sinks must be defensive and should not panic.
- A failing sink should not break rendering unless the user explicitly opts into
  strict behavior.
- Fields should be structured and boring: strings, ints, bools, durations,
  errors.
- Avoid putting large HTML payloads in events.

## Migration State

1. Core has `Event`, `EventLevel`, `EventSink`, `EventSinkFunc`, async dispatch, and fanout.
2. `Partial.SetEvents` propagates into `RenderContext`.
3. Core render/template/target failures emit diagnostic events instead of calling a logger.
4. `ext/logger` consumes events through `slog`.
5. `exp/metrics` can consume events, though the showcase keeps render metrics and diagnostic logs separate.
6. The showcase exposes metrics pages for measurements and a logger page for diagnostic events.
