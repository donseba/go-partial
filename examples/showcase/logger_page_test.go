package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/metrics"
)

func TestShowcaseLogsKeepsLatestRecords(t *testing.T) {
	logs := newShowcaseLogs(2)
	req := httptest.NewRequest("GET", "/logger", nil)
	ctx := &partial.RenderContext{
		Context: metrics.WithTraceID(context.Background(), "trace-test"),
		Request: req,
	}

	logs.Emit(ctx, partial.Event{Time: time.Unix(1, 0), Kind: "first", Level: partial.EventInfo})
	logs.Emit(ctx, partial.Event{Time: time.Unix(2, 0), Kind: "second", Level: partial.EventWarn})
	logs.Emit(ctx, partial.Event{Time: time.Unix(3, 0), Kind: "third", Level: partial.EventError})

	records, total := logs.Snapshot(10)
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if records[0].event.Kind != "third" || records[1].event.Kind != "second" {
		t.Fatalf("records = %#v, want latest first", records)
	}

	views := logRecordViews(records, partial.EventInfo)
	if views[0].Request != "GET /logger" {
		t.Fatalf("request = %q, want GET /logger", views[0].Request)
	}
	if views[0].TraceID != "trace-test" {
		t.Fatalf("trace = %q, want trace-test", views[0].TraceID)
	}
}

func TestLogRecordViewsFilterDebugByDefault(t *testing.T) {
	records := []showcaseLogRecord{
		{event: partial.Event{Kind: "debug", Level: partial.EventDebug}},
		{event: partial.Event{Kind: "info", Level: partial.EventInfo}},
	}

	views := logRecordViews(records, partial.EventInfo)
	if len(views) != 1 || views[0].Kind != "info" {
		t.Fatalf("views = %#v, want only info event", views)
	}

	views = logRecordViews(records, partial.EventDebug)
	if len(views) != 2 {
		t.Fatalf("views = %d, want 2", len(views))
	}
}
