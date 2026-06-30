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
service.Use(logger.Renderer())
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

OpenTelemetry is a good fit as an external sink because events already describe
facts. A sink can add go-partial events to the active span:

```go
type OTelSink struct{}

func (OTelSink) Emit(ctx *partial.RenderContext, event partial.Event) {
    if ctx == nil || ctx.Context == nil {
        return
    }
    span := trace.SpanFromContext(ctx.Context)
    if !span.IsRecording() {
        return
    }

    attrs := []attribute.KeyValue{
        attribute.String("partial.event", event.Kind),
        attribute.String("partial.level", string(event.Level)),
        attribute.String("partial.id", event.PartialID),
        attribute.String("partial.parent", event.ParentID),
        attribute.String("partial.name", event.Name),
    }
    for key, value := range event.Fields {
        attrs = append(attrs, attribute.String("partial."+key, fmt.Sprint(value)))
    }
    if event.Error != nil {
        span.RecordError(event.Error)
    }
    span.AddEvent(event.Message, trace.WithAttributes(attrs...))
}
```

In an HTTP app, create the request span in middleware and render with the request
context. go-partial does not create spans by itself; it only emits events into
the context you provide.

## Metrics

Use `exp/metrics` when you need render measurements:

```go
sink := metrics.Fanout(
    inMemoryStore,
    metrics.NewWriterSink(os.Stdout),
)

service.Use(metrics.Renderer(
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
