// Package sse provides experimental server-sent event helpers for streaming rendered partials.
package sse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
)

type (
	HeaderKey   string
	HeaderValue string
	EventName   string

	// Writer streams server-sent events to one http.ResponseWriter.
	//
	// Writer is bound to a single response stream and is not safe for concurrent
	// writes unless the caller serializes access.
	Writer struct {
		w         http.ResponseWriter
		flusher   http.Flusher
		renderers []partial.Renderer
	}

	Patch struct {
		Target string        `json:"target"`
		HTML   template.HTML `json:"html"`
	}

	Signal struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	}
)

const (
	HeaderContentType     HeaderKey = "Content-Type"
	HeaderCacheControl    HeaderKey = "Cache-Control"
	HeaderConnection      HeaderKey = "Connection"
	HeaderXAccelBuffering HeaderKey = "X-Accel-Buffering"
)

const (
	ContentTypeEventStream HeaderValue = "text/event-stream"
	CacheControlNoCache    HeaderValue = "no-cache"
	ConnectionKeepAlive    HeaderValue = "keep-alive"
	XAccelBufferingNo      HeaderValue = "no"
)

const (
	EventPatch  EventName = "partial:patch"
	EventSignal EventName = "partial:signal"
	EventError  EventName = "partial:error"
)

func (h HeaderKey) String() string {
	return string(h)
}

func (v HeaderValue) String() string {
	return string(v)
}

func (e EventName) String() string {
	return string(e)
}

func NewWriter(w http.ResponseWriter) *Writer {
	writer := &Writer{w: w}
	if flusher, ok := w.(http.Flusher); ok {
		writer.flusher = flusher
	}
	writer.ApplyHeaders()
	return writer
}

// Use appends renderers to partials rendered by this writer.
func (s *Writer) Use(renderers ...partial.Renderer) *Writer {
	if s == nil {
		return s
	}
	s.renderers = append(s.renderers, renderers...)
	return s
}

func (s *Writer) ApplyHeaders() {
	if s == nil || s.w == nil {
		return
	}
	headers := s.w.Header()
	headers.Set(HeaderContentType.String(), ContentTypeEventStream.String())
	headers.Set(HeaderCacheControl.String(), CacheControlNoCache.String())
	headers.Set(HeaderConnection.String(), ConnectionKeepAlive.String())
	headers.Set(HeaderXAccelBuffering.String(), XAccelBufferingNo.String())
}

func (s *Writer) Comment(comment string) error {
	if s == nil || s.w == nil {
		return fmt.Errorf("sse writer is not initialized")
	}
	for _, line := range splitSSELines(comment) {
		if _, err := fmt.Fprintf(s.w, ": %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(s.w, "\n")
	return err
}

func (s *Writer) Retry(after time.Duration) error {
	if s == nil || s.w == nil {
		return fmt.Errorf("sse writer is not initialized")
	}
	_, err := fmt.Fprintf(s.w, "retry: %d\n\n", after.Milliseconds())
	return err
}

func (s *Writer) Event(event EventName, data any) error {
	return s.EventID("", event, data)
}

func (s *Writer) EventID(id string, event EventName, data any) error {
	if s == nil || s.w == nil {
		return fmt.Errorf("sse writer is not initialized")
	}
	if id != "" {
		if _, err := fmt.Fprintf(s.w, "id: %s\n", sanitizeSSEField(id)); err != nil {
			return err
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(s.w, "event: %s\n", sanitizeSSEField(event.String())); err != nil {
			return err
		}
	}
	payload, err := encodeSSEData(data)
	if err != nil {
		return err
	}
	for _, line := range splitSSELines(payload) {
		if _, err = fmt.Fprintf(s.w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(s.w, "\n")
	return err
}

func (s *Writer) PatchHTML(target string, html template.HTML) error {
	return s.Event(EventPatch, Patch{
		Target: target,
		HTML:   html,
	})
}

func (s *Writer) PatchPartial(ctx context.Context, r *http.Request, target string, p *partial.Partial) error {
	if p == nil {
		return fmt.Errorf("partial is not initialized")
	}
	if len(s.renderers) > 0 {
		p = p.Clone()
		p.Use(s.renderers...)
	}
	html, err := p.RenderWithRequest(ctx, r)
	if err != nil {
		return err
	}
	return s.PatchHTML(target, html)
}

func (s *Writer) Signal(name string, value any) error {
	return s.Event(EventSignal, Signal{
		Name:  name,
		Value: value,
	})
}

func (s *Writer) Error(err error) error {
	if err == nil {
		return nil
	}
	return s.Event(EventError, map[string]string{
		"error": err.Error(),
	})
}

func (s *Writer) Flush() {
	if s == nil || s.flusher == nil {
		return
	}
	s.flusher.Flush()
}

func encodeSSEData(data any) (string, error) {
	switch v := data.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case template.HTML:
		return string(v), nil
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func splitSSELines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Split(value, "\n")
}

func sanitizeSSEField(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}
