package main

import (
	"fmt"
	"net/http"
	"sort"
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
		TraceLabel: "request + trace id",
	}
	app.render(w, r, "content", "templates/metrics.gohtml", data)
}

func metricsRecordViews(records []metrics.Record) []MetricsTraceView {
	groupRecords := make(map[string][]metrics.Record)
	for _, record := range records {
		if isMetricEventRecord(record) {
			continue
		}
		requestID := shortRequestID(record.RequestID)
		if requestID == "" {
			requestID = "-"
		}
		groupRecords[requestID] = append(groupRecords[requestID], record)
	}

	traces := make([]metricTraceRecords, 0, len(groupRecords))
	for requestID, records := range groupRecords {
		records = sortedMetricStack(records)
		first := records[0]
		latest := latestMetricTimestamp(records)
		group := MetricsTraceView{
			RequestID: requestID,
			TraceID:   shortRequestID(first.TraceID),
			Timestamp: formatTimestamp(latest),
			Method:    first.Method,
			Path:      first.Path,
			Records:   make([]MetricsRecordView, 0, len(records)),
		}
		for _, record := range records {
			group.Records = append(group.Records, MetricsRecordView{
				Kind:            metricKind(record),
				Name:            record.Name,
				RequestID:       requestID,
				TraceID:         shortRequestID(record.TraceID),
				ParentRequestID: shortRequestID(record.ParentRequestID),
				Timestamp:       formatTimestamp(record.StartedAt),
				Label:           metricLabel(record),
				PartialID:       record.PartialID,
				ParentID:        record.ParentID,
				Depth:           metricDepth(record, records),
				Indent:          metricIndent(metricDepth(record, records)),
				Meta:            metricMeta(record),
				PartialLabel:    record.PartialLabel,
				SlotName:        record.SlotName,
				Templates:       strings.Join(record.Templates, ", "),
				Swap:            formatMetricSwap(record.OOB),
				Method:          record.Method,
				Path:            record.Path,
				Size:            formatBytes(record.Size),
				Duration:        formatDuration(record.Duration),
				Error:           formatMetricError(record.Error),
				Chain:           record.Tags["chain"],
			})
		}
		traces = append(traces, metricTraceRecords{Latest: latest, View: group})
	}

	sort.SliceStable(traces, func(i, j int) bool {
		return traces[i].Latest.After(traces[j].Latest)
	})
	groups := make([]MetricsTraceView, 0, len(traces))
	for _, trace := range traces {
		groups = append(groups, trace.View)
	}
	return groups
}

type metricTraceRecords struct {
	Latest time.Time
	View   MetricsTraceView
}

func sortedMetricStack(records []metrics.Record) []metrics.Record {
	out := append([]metrics.Record(nil), records...)
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		leftDepth := metricDepth(left, out)
		rightDepth := metricDepth(right, out)
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if !left.StartedAt.Equal(right.StartedAt) {
			return left.StartedAt.Before(right.StartedAt)
		}
		return left.PartialID < right.PartialID
	})
	return out
}

func latestMetricTimestamp(records []metrics.Record) time.Time {
	var latest time.Time
	for _, record := range records {
		if record.StartedAt.After(latest) {
			latest = record.StartedAt
		}
	}
	return latest
}

func metricDepth(record metrics.Record, records []metrics.Record) int {
	if record.ParentID == "" {
		return 0
	}
	seen := make(map[string]struct{})
	depth := 0
	parentID := record.ParentID
	for parentID != "" {
		if _, ok := seen[parentID]; ok {
			return depth
		}
		seen[parentID] = struct{}{}
		depth++
		nextParentID := ""
		for _, candidate := range records {
			if candidate.RequestID == record.RequestID && candidate.PartialID == parentID {
				nextParentID = candidate.ParentID
				break
			}
		}
		parentID = nextParentID
	}
	return depth
}

func metricLabel(record metrics.Record) string {
	if record.PartialLabel != "" {
		return record.PartialLabel
	}
	if record.Kind == "interaction" && record.Name != "" {
		return record.Name
	}
	if record.PartialID != "" {
		return record.PartialID
	}
	return string(record.Kind)
}

func metricKind(record metrics.Record) string {
	if record.SlotName != "" {
		return "slot"
	}
	return string(record.Kind)
}

func metricMeta(record metrics.Record) []string {
	var meta []string
	if record.PartialID != "" {
		meta = append(meta, "partial: "+record.PartialID)
	}
	if record.SlotName != "" {
		meta = append(meta, "slot: "+record.SlotName)
	}
	if record.Name != "" {
		meta = append(meta, "task: "+record.Name)
	}
	return meta
}

func isMetricEventRecord(record metrics.Record) bool {
	return record.Kind == "event" || record.EventKind != ""
}

func metricIndent(depth int) string {
	switch {
	case depth <= 0:
		return ""
	case depth == 1:
		return "pl-6"
	default:
		return "pl-6"
	}
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

func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Format("15:04:05.000")
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
