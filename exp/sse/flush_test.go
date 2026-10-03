package sse

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type unwrapWriter struct{ http.ResponseWriter }

func (w unwrapWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestFlushThroughMiddleware(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewWriter(unwrapWriter{unwrapWriter{rec}})
	if err := writer.Signal("ready", true); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if !rec.Flushed || !strings.Contains(rec.Body.String(), "partial:signal") {
		t.Fatalf("wrapped SSE response was not flushed: %s", rec.Body.String())
	}
}

type errorFlusher struct {
	http.ResponseWriter
	flushed bool
}

func (w *errorFlusher) FlushError() error {
	w.flushed = true
	return http.ErrNotSupported
}

func TestFlushHonorsWrapperFlushError(t *testing.T) {
	wrapper := &errorFlusher{ResponseWriter: httptest.NewRecorder()}
	NewWriter(wrapper).Flush()
	if !wrapper.flushed {
		t.Fatal("FlushError middleware was bypassed")
	}
}

func TestFlushUninitializedWriter(t *testing.T) {
	var writer *Writer
	writer.Flush()
	NewWriter(nil).Flush()
}
