package partial

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func TestAsyncEventsEmitDoesNotBlockOnSlowSink(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	sink := EventSinkFunc(func(ctx *RenderContext, event Event) {
		close(started)
		<-release
	})
	events := NewAsyncEvents(EventsConfig{Buffer: 1}, sink)
	defer func() {
		close(release)
		_ = events.Close(context.Background())
	}()

	events.Emit(nil, Event{Kind: "test.first"})
	<-started

	done := make(chan struct{})
	go func() {
		for range 100 {
			events.Emit(nil, Event{Kind: "test.next"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Emit blocked on a slow sink")
	}
}

func TestRenderContextEmitFillsIdentity(t *testing.T) {
	var got Event
	p := NewID("content", "content.gohtml")
	parent := NewID("layout", "layout.gohtml").With(p)
	p.parent = parent
	ctx := newRenderContext(context.Background(), p, nil, RenderKindPartial)
	ctx.Events = EventSinkFunc(func(ctx *RenderContext, event Event) {
		got = event
	})

	ctx.Emit(Event{Kind: "test.identity"})

	if got.Time.IsZero() {
		t.Fatal("Time was not set")
	}
	if got.Level != EventInfo {
		t.Fatalf("Level = %q, want %q", got.Level, EventInfo)
	}
	if got.PartialID != "content" || got.ParentID != "layout" {
		t.Fatalf("identity = partial %q parent %q", got.PartialID, got.ParentID)
	}
}

func TestFanoutEventsSkipsNilAndRecoversPanics(t *testing.T) {
	var count atomic.Int64
	sink := FanoutEvents(
		nil,
		EventSinkFunc(func(ctx *RenderContext, event Event) {
			panic("boom")
		}),
		EventSinkFunc(func(ctx *RenderContext, event Event) {
			count.Add(1)
		}),
	)

	sink.Emit(nil, Event{Kind: "test.fanout"})

	if count.Load() != 1 {
		t.Fatalf("count = %d, want 1", count.Load())
	}
}

func TestRenderEmitsLifecycleEvents(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`hello {{ .Name }}`)},
	}
	var events []Event
	page := NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetDot(map[string]string{"Name": "world"}).
		SetEventSink(EventSinkFunc(func(ctx *RenderContext, event Event) {
			events = append(events, event)
		}))

	html, err := page.RenderWithRequest(context.Background(), httptestRequest("GET", "/page"))
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if html != "hello world" {
		t.Fatalf("html = %q, want %q", html, "hello world")
	}

	if !hasEvent(events, EventRenderStart) {
		t.Fatalf("missing %s event: %#v", EventRenderStart, events)
	}
	if !hasEvent(events, EventRenderFinish) {
		t.Fatalf("missing %s event: %#v", EventRenderFinish, events)
	}
}

func TestRequestContextEventSinkReceivesLifecycleEvents(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`hello`)},
	}
	var events []Event
	ctx := WithEventSink(context.Background(), EventSinkFunc(func(ctx *RenderContext, event Event) {
		events = append(events, event)
	}))
	page := NewID("page", "page.gohtml").SetFileSystem(files)

	if _, err := page.RenderWithRequest(ctx, httptestRequest("GET", "/page")); err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if !hasEvent(events, EventRenderStart) || !hasEvent(events, EventRenderFinish) {
		t.Fatalf("request context events = %#v, want lifecycle events", events)
	}
}

func TestRequestContextEventSinkFansOutWithPartialSink(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`hello`)},
	}
	var partialCount atomic.Int64
	var requestCount atomic.Int64
	ctx := WithEventSink(context.Background(), EventSinkFunc(func(ctx *RenderContext, event Event) {
		if event.Kind == EventRenderFinish {
			requestCount.Add(1)
		}
	}))
	page := NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetEventSink(EventSinkFunc(func(ctx *RenderContext, event Event) {
			if event.Kind == EventRenderFinish {
				partialCount.Add(1)
			}
		}))

	if _, err := page.RenderWithRequest(ctx, httptestRequest("GET", "/page")); err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if partialCount.Load() != 1 || requestCount.Load() != 1 {
		t.Fatalf("partialCount = %d requestCount = %d, want 1/1", partialCount.Load(), requestCount.Load())
	}
}

func TestWithEventSinkAppendsRequestScopedSinks(t *testing.T) {
	var first atomic.Int64
	var second atomic.Int64
	ctx := WithEventSink(context.Background(), EventSinkFunc(func(ctx *RenderContext, event Event) {
		first.Add(1)
	}))
	ctx = WithEventSink(ctx, EventSinkFunc(func(ctx *RenderContext, event Event) {
		second.Add(1)
	}))

	EventSinkFromContext(ctx).Emit(nil, Event{Kind: "test"})

	if first.Load() != 1 || second.Load() != 1 {
		t.Fatalf("first = %d second = %d, want 1/1", first.Load(), second.Load())
	}
}

func httptestRequest(method, target string) *http.Request {
	req, _ := http.NewRequest(method, target, nil)
	return req
}

func hasEvent(events []Event, kind string) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}
