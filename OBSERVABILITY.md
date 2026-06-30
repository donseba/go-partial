# Observability

go-partial keeps rendering independent from logging, metrics, queues, and tracing.
Core emits small diagnostic events, and applications choose where those events go.

There are two separate observability paths:

- `exp/metrics` measures render work: duration, size, request IDs, trace IDs,
  slots, partial labels, and render tree shape.
- `partial.Event` records diagnostic facts: render lifecycle, failures, missing
  templates, skipped protected functions, target misses, and extension signals.

The showcase keeps these separate: `/metrics` shows render measurements, while
`/logger` shows diagnostic events.

## Attach Events

Attach event consumers at the service boundary:

```go
events := partial.NewAsyncEvents(
    partial.EventsConfig{
        Buffer:     256,
        DropPolicy: partial.DropNewest,
    },
    logger.Sink(slog.Default(), logger.WithMinLevel(partial.EventWarn)),
    mySink,
)

service := partial.NewService(&partial.Config{
    Events: events,
})
```

`NewAsyncEvents` is the recommended default. It uses a bounded queue so slow
consumers do not block a short render. When the queue is full, `DropNewest`
drops the newest event and increments the dropped counter.

Use debug-level events for local trace views. Use `warn` or `error` for normal
production logs:

```go
logger.Sink(slog.Default(), logger.WithMinLevel(partial.EventWarn))
```

## Request-Scoped Events

You can also attach an event sink to a request context:

```go
func observability(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        events := partial.NewAsyncEvents(
            partial.EventsConfig{Buffer: 64},
            otel.Sink(otel.WithMinLevel(partial.EventInfo)),
        )
        defer func() {
            ctx, cancel := context.WithTimeout(context.Background(), time.Second)
            defer cancel()
            _ = events.Close(ctx)
        }()

        ctx := partial.WithEventSink(r.Context(), events)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Renders started with that context fan out to both the request-scoped sink and
the service/partial sink, if one is configured.

Calling `WithEventSink` more than once appends sinks; it does not replace the
previous request-scoped sink.

Do not close a shared service-level dispatcher from a request. A service-level
`AsyncEvents` belongs to the server lifecycle. A request-level `AsyncEvents`
belongs to the request/middleware lifecycle.

## Stdout Or JSON Logs

`ext/logger` adapts events to `log/slog`, so stdout, JSON logs, files, and any
custom slog handler work without core knowing about them:

```go
log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

events := partial.NewAsyncEvents(
    partial.EventsConfig{Buffer: 256},
    logger.Sink(log, logger.WithMinLevel(partial.EventInfo)),
)
```

For production, prefer a higher minimum level:

```go
logger.Sink(log, logger.WithMinLevel(partial.EventWarn))
```

## Template Logging

`ext/logger` can also install a template helper for lightweight diagnostic
breadcrumbs:

```go
service.SetFunc(logger.FuncMap())
service.Use(logger.Stage())
```

Templates can then emit an info-level event:

```gotemplate
{{ logger "cart sidebar rendered" "items" .Cart.Count "template" "cart.gohtml" }}
```

The helper returns an empty string, so it does not render visible HTML. It emits
an `ext.logger.template` event with structured fields. Treat it as a diagnostic
breadcrumb only; templates should not use logging to control response behavior.

## Multiple Consumers

Event sinks are peers. You can send the same event to stdout, an in-memory
debug page, a queue, and telemetry:

```go
events := partial.NewAsyncEvents(
    partial.EventsConfig{Buffer: 512},
    logger.Sink(log, logger.WithMinLevel(partial.EventWarn)),
    appDebugStore,
    dynamoSink,
    kafkaSink,
    otelSink,
)
```

The same can be written explicitly with `partial.FanoutEvents(...)`, but
`NewAsyncEvents` already accepts multiple sinks.

## DynamoDB, Kafka, Or Other Stores

External stores should be app-owned sinks. Keep the sink cheap: convert the
event into your storage shape and enqueue it to your own client/worker.

```go
type DynamoSink struct {
    client *dynamodb.Client
    table  string
}

func (sink DynamoSink) Emit(ctx *partial.RenderContext, event partial.Event) {
    item := map[string]types.AttributeValue{
        "kind":    &types.AttributeValueMemberS{Value: event.Kind},
        "level":   &types.AttributeValueMemberS{Value: string(event.Level)},
        "message": &types.AttributeValueMemberS{Value: event.Message},
        "partial": &types.AttributeValueMemberS{Value: event.PartialID},
    }

    go func() {
        _, _ = sink.client.PutItem(context.Background(), &dynamodb.PutItemInput{
            TableName: aws.String(sink.table),
            Item:      item,
        })
    }()
}
```

For Kafka or NATS, the same pattern applies:

```go
kafkaSink := partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
    payload, _ := json.Marshal(eventDTO(event))
    _ = producer.Produce(&kafka.Message{
        TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
        Value:          payload,
    }, nil)
})
```

If your producer can block, keep it behind its own queue or use
`partial.NewAsyncEvents` with a drop policy that fits the application.

## OpenTelemetry

OpenTelemetry is a good fit because events already describe facts.
`exp/otel` adds go-partial events to the active span in the render context:

```go
events := partial.NewAsyncEvents(
    partial.EventsConfig{Buffer: 256},
    otel.Sink(otel.WithMinLevel(partial.EventInfo)),
)
```

In an HTTP app, create the request span in middleware and render with the request
context. go-partial does not create spans by itself; it only emits events into
the context you provide.

`exp/otel` writes attributes with the `partial.` prefix by default, such as
`partial.event`, `partial.level`, `partial.partial_id`, and `partial.size`.
Use `otel.WithAttributePrefix("go_partial.")` if your telemetry naming scheme
needs a different prefix.

You can still write your own `partial.EventSink` when you need a different span
shape, exporter policy, or attribute naming convention.

## Metrics

Use `exp/metrics` when you need render measurements:

```go
sink := metrics.Fanout(
    inMemoryStore,
    metrics.NewWriterSink(os.Stdout),
)

service.Use(metrics.Stage(
    sink,
    metrics.WithTag("chain", "web"),
    metrics.WithSlotName(slots.Name),
))
```

Metrics and logs can share request IDs and trace IDs through context:

```go
ctx := metrics.WithRequestID(r.Context(), requestID)
ctx = metrics.WithTraceID(ctx, traceID)
_ = layout.WriteWithRequest(ctx, w, r)
```

That lets `/metrics`, `/logger`, stdout logs, and external telemetry correlate
without coupling core to any backend.

## Core Event Kinds

| Kind | Level | Meaning |
| --- | --- | --- |
| `render.start` | `debug` | A render lifecycle started. |
| `render.finish` | `debug` | A render lifecycle finished successfully. Includes `size`. |
| `render.error` | `error` | Rendering failed. Includes the error. |
| `render.write_error` | `error` | Writing the render response failed. |
| `render.oob_error` | `error` | Rendering an out-of-band child failed. |
| `template.missing` | `warn` | A template helper referenced a missing template. |
| `template.parse_error` | `error` | Template parsing or cache lookup failed. |
| `template.execute_error` | `error` | Template execution failed. |
| `func.protected` | `warn` | A user function tried to overwrite a protected helper. |
| `content.missing_layout` | `warn` | `content` was called outside a layout wrapper. |
| `target.missing` | `warn` | A requested target could not be resolved. |
| `contract.invalid` | `warn` | Template contract data or helper arguments were invalid. |
