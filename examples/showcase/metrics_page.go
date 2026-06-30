package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/donseba/go-partial/exp/metrics"
)

type showcaseMetrics struct {
	mu      sync.Mutex
	limit   int
	records []metrics.Record
}

func newShowcaseMetrics(limit int) *showcaseMetrics {
	return &showcaseMetrics{limit: limit}
}

func (store *showcaseMetrics) Record(record metrics.Record) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	store.records = append([]metrics.Record{record}, store.records...)
	if store.limit > 0 && len(store.records) > store.limit {
		store.records = store.records[:store.limit]
	}
}

func (store *showcaseMetrics) Snapshot(limit int) ([]metrics.Record, int) {
	if store == nil {
		return nil, 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	total := len(store.records)
	if limit <= 0 || limit > len(store.records) {
		limit = len(store.records)
	}
	out := append([]metrics.Record(nil), store.records[:limit]...)
	return out, total
}

func (app *App) metricsPage(w http.ResponseWriter, r *http.Request) {
	records, total := app.metrics.Snapshot(18)
	data := MetricsPage{
		Title:      "Render metrics",
		Total:      total,
		Latest:     metricsRecordViews(records),
		ChainTag:   "showcase",
		TraceLabel: "request id",
	}
	app.render(w, r, "content", "templates/metrics.gohtml", data)
}

func metricsRecordViews(records []metrics.Record) []MetricsRecordView {
	views := make([]MetricsRecordView, 0, len(records))
	for _, record := range records {
		views = append(views, MetricsRecordView{
			Kind:       string(record.Kind),
			Name:       record.Name,
			RequestID:  shortRequestID(record.RequestID),
			PartialID:  record.PartialID,
			PartialTag: record.PartialTag,
			Templates:  strings.Join(record.Templates, ", "),
			Swap:       formatMetricSwap(record.OOB),
			Method:     record.Method,
			Path:       record.Path,
			Size:       formatBytes(record.Size),
			Duration:   formatDuration(record.Duration),
			Error:      formatMetricError(record.Error),
			Chain:      record.Tags["chain"],
		})
	}
	return views
}

func formatBytes(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func formatDuration(duration time.Duration) string {
	if duration < time.Microsecond {
		return duration.String()
	}
	if duration < time.Millisecond {
		return fmt.Sprintf("%.1f us", float64(duration)/float64(time.Microsecond))
	}
	return fmt.Sprintf("%.2f ms", float64(duration)/float64(time.Millisecond))
}

func formatMetricError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func shortRequestID(requestID string) string {
	if len(requestID) <= 10 {
		return requestID
	}
	return requestID[:10]
}

func formatMetricSwap(oob bool) string {
	if oob {
		return "oob"
	}
	return "-"
}
