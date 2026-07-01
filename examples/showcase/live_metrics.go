package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/metrics"
	"github.com/donseba/go-partial/exp/sse"
)

type metricStreamHub struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[int]chan metrics.Record
}

func newMetricStreamHub() *metricStreamHub {
	return &metricStreamHub{subscribers: make(map[int]chan metrics.Record)}
}

func (hub *metricStreamHub) Record(record metrics.Record) {
	if hub == nil {
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for _, subscriber := range hub.subscribers {
		select {
		case subscriber <- record:
		default:
		}
	}
}

func (hub *metricStreamHub) subscribe() (<-chan metrics.Record, func()) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.nextID++
	id := hub.nextID
	records := make(chan metrics.Record, 32)
	hub.subscribers[id] = records
	return records, func() {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		if subscriber, ok := hub.subscribers[id]; ok {
			delete(hub.subscribers, id)
			close(subscriber)
		}
	}
}

func (app *App) liveMetricsPage(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/live_metrics.gohtml", LiveMetricsPage{
		Title: "Live render metrics",
	})
}

func (app *App) liveMetricsPing(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("live-metrics-ping", "templates/live_metrics_ping.gohtml").
		SetDot(LiveMetricPing{Time: time.Now().Format("15:04:05.000")})
	app.writeStandalone(w, r, content)
}

func (app *App) liveMetricsStream(w http.ResponseWriter, r *http.Request) {
	events := sse.NewWriter(w)
	_ = events.Comment("go-partial live metrics")
	events.Flush()

	records, unsubscribe := app.metricStreams.subscribe()
	defer unsubscribe()

	for {
		select {
		case <-r.Context().Done():
			return
		case record, ok := <-records:
			if !ok {
				return
			}
			if record.Path == "/metrics/live/stream" || isMetricEventRecord(record) {
				continue
			}
			html, err := app.renderLiveMetricRow(r.Context(), record)
			if err != nil {
				_ = events.Error(err)
				events.Flush()
				continue
			}
			if err = events.Event(sse.EventPatch, html); err != nil {
				return
			}
			events.Flush()
		}
	}
}

func (app *App) renderLiveMetricRow(ctx context.Context, record metrics.Record) (template.HTML, error) {
	row := partial.NewID("live-metric-row", "templates/live_metric_row.gohtml").
		SetFileSystem(os.DirFS("examples/showcase")).
		SetDot(liveMetricRow(record))
	return partial.Render(ctx, row)
}

func liveMetricRow(record metrics.Record) LiveMetricRow {
	return LiveMetricRow{
		Timestamp: record.StartedAt.Format("15:04:05.000"),
		Kind:      metricKind(record),
		Label:     metricLabel(record),
		Meta:      metricMeta(record),
		Templates: strings.Join(record.Templates, ", "),
		Swap:      formatMetricSwap(record.OOB),
		Method:    record.Method,
		Path:      record.Path,
		Duration:  formatDuration(record.Duration),
		Size:      formatBytes(record.Size),
		Chain:     record.Tags["chain"],
		Error:     metricError(record),
	}
}

func metricError(record metrics.Record) string {
	if record.Error == nil {
		return ""
	}
	return fmt.Sprintf("%v", record.Error)
}
