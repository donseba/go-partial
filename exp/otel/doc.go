// Package otel adds go-partial's diagnostic events to the active
// OpenTelemetry span of the render's context, through the event sink of
// Sink. It creates no spans, tracer providers or exporters: the application
// starts the span, such as in its HTTP middleware, and renders with that
// context.
package otel
