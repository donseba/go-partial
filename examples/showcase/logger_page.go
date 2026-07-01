package main

import (
	"fmt"
	"net/http"
	"sort"
	"sync"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/metrics"
)

type showcaseLogRecord struct {
	event   partial.Event
	method  string
	path    string
	traceID string
}

type showcaseLogs struct {
	mu      sync.Mutex
	limit   int
	records []showcaseLogRecord
}

func newShowcaseLogs(limit int) *showcaseLogs {
	return &showcaseLogs{limit: limit}
}

func (logs *showcaseLogs) Emit(ctx *partial.RenderContext, event partial.Event) {
	if logs == nil {
		return
	}
	record := showcaseLogRecord{event: event}
	if ctx != nil {
		record.traceID = metrics.TraceIDFromContext(ctx.Context)
		if ctx.Request != nil {
			record.method = ctx.Request.Method
			if ctx.Request.URL != nil {
				record.path = ctx.Request.URL.Path
			}
		}
	}

	logs.mu.Lock()
	defer logs.mu.Unlock()

	logs.records = append([]showcaseLogRecord{record}, logs.records...)
	if logs.limit > 0 && len(logs.records) > logs.limit {
		logs.records = logs.records[:logs.limit]
	}
}

func (logs *showcaseLogs) Snapshot(limit int) ([]showcaseLogRecord, int) {
	if logs == nil {
		return nil, 0
	}
	logs.mu.Lock()
	defer logs.mu.Unlock()

	total := len(logs.records)
	if limit <= 0 || limit > len(logs.records) {
		limit = len(logs.records)
	}
	out := append([]showcaseLogRecord(nil), logs.records[:limit]...)
	return out, total
}

func (app *App) loggerPage(w http.ResponseWriter, r *http.Request) {
	filter := loggerLevelFilter(r)
	records, total := app.logs.Snapshot(120)
	views := logRecordViews(records, filter)
	data := LoggerPage{
		Title:       "Diagnostic logger",
		Total:       total,
		Visible:     len(views),
		Latest:      limitLogViews(views, 24),
		LevelFilter: string(filter),
		DebugURL:    "/logger?level=debug",
		InfoURL:     "/logger",
	}
	app.render(w, r, "content", "templates/logger.gohtml", data)
}

func logRecordViews(records []showcaseLogRecord, minLevel partial.EventLevel) []LogRecordView {
	out := make([]LogRecordView, 0, len(records))
	for _, record := range records {
		event := record.event
		if eventLevelRank(event.Level) < eventLevelRank(minLevel) {
			continue
		}
		out = append(out, LogRecordView{
			Timestamp: formatTimestamp(event.Time),
			Level:     string(event.Level),
			Kind:      event.Kind,
			Message:   event.Message,
			PartialID: event.PartialID,
			Request:   formatLogRequest(record.method, record.path),
			TraceID:   shortRequestID(record.traceID),
			Fields:    formatLogFields(event.Fields),
			Error:     formatEventError(event.Error),
		})
	}
	return out
}

func loggerLevelFilter(r *http.Request) partial.EventLevel {
	if r != nil && r.URL != nil && r.URL.Query().Get("level") == "debug" {
		return partial.EventDebug
	}
	return partial.EventInfo
}

func limitLogViews(views []LogRecordView, limit int) []LogRecordView {
	if limit <= 0 || len(views) <= limit {
		return views
	}
	return views[:limit]
}

func eventLevelRank(level partial.EventLevel) int {
	switch level {
	case partial.EventDebug:
		return 0
	case partial.EventInfo:
		return 1
	case partial.EventWarn:
		return 2
	case partial.EventError:
		return 3
	default:
		return 1
	}
}

func formatLogRequest(method, path string) string {
	if method == "" && path == "" {
		return "-"
	}
	return method + " " + path
}

func formatLogFields(fields map[string]any) []string {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s: %v", key, fields[key]))
	}
	return out
}

func formatEventError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
