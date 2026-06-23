package partial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

type (
	SSEHeaderKey   string
	SSEHeaderValue string
	SSEEventName   string

	SSEWriter struct {
		w       http.ResponseWriter
		flusher http.Flusher
	}

	SSEPatch struct {
		Target string        `json:"target"`
		HTML   template.HTML `json:"html"`
	}

	SSESignal struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	}
)

const (
	SSEHeaderContentType      SSEHeaderKey = "Content-Type"
	SSEHeaderCacheControl     SSEHeaderKey = "Cache-Control"
	SSEHeaderConnection       SSEHeaderKey = "Connection"
	SSEHeaderXAccelBuffering  SSEHeaderKey = "X-Accel-Buffering"
	SSEHeaderTransferEncoding SSEHeaderKey = "Transfer-Encoding"
)

const (
	SSEContentTypeEventStream  SSEHeaderValue = "text/event-stream"
	SSECacheControlNoCache     SSEHeaderValue = "no-cache"
	SSEConnectionKeepAlive     SSEHeaderValue = "keep-alive"
	SSEXAccelBufferingNo       SSEHeaderValue = "no"
	SSETransferEncodingChunked SSEHeaderValue = "chunked"
)

const (
	SSEEventPatch  SSEEventName = "partial:patch"
	SSEEventSignal SSEEventName = "partial:signal"
	SSEEventError  SSEEventName = "partial:error"
)

func (h SSEHeaderKey) String() string {
	return string(h)
}

func (v SSEHeaderValue) String() string {
	return string(v)
}

func (e SSEEventName) String() string {
	return string(e)
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	writer := &SSEWriter{w: w}
	if flusher, ok := w.(http.Flusher); ok {
		writer.flusher = flusher
	}
	writer.ApplyHeaders()
	return writer
}

func (s *SSEWriter) ApplyHeaders() {
	if s == nil || s.w == nil {
		return
	}
	headers := s.w.Header()
	headers.Set(SSEHeaderContentType.String(), SSEContentTypeEventStream.String())
	headers.Set(SSEHeaderCacheControl.String(), SSECacheControlNoCache.String())
	headers.Set(SSEHeaderConnection.String(), SSEConnectionKeepAlive.String())
	headers.Set(SSEHeaderXAccelBuffering.String(), SSEXAccelBufferingNo.String())
}

func (s *SSEWriter) Comment(comment string) error {
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

func (s *SSEWriter) Retry(after time.Duration) error {
	if s == nil || s.w == nil {
		return fmt.Errorf("sse writer is not initialized")
	}
	_, err := fmt.Fprintf(s.w, "retry: %d\n\n", after.Milliseconds())
	return err
}

func (s *SSEWriter) Event(event SSEEventName, data any) error {
	return s.EventID("", event, data)
}

func (s *SSEWriter) EventID(id string, event SSEEventName, data any) error {
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

func (s *SSEWriter) PatchHTML(target string, html template.HTML) error {
	return s.Event(SSEEventPatch, SSEPatch{
		Target: target,
		HTML:   html,
	})
}

func (s *SSEWriter) PatchPartial(ctx context.Context, r *http.Request, target string, p *Partial) error {
	if p == nil {
		return fmt.Errorf("partial is not initialized")
	}
	html, err := p.RenderWithRequest(ctx, r)
	if err != nil {
		return err
	}
	return s.PatchHTML(target, html)
}

func (s *SSEWriter) Signal(name string, value any) error {
	return s.Event(SSEEventSignal, SSESignal{
		Name:  name,
		Value: value,
	})
}

func (s *SSEWriter) Error(err error) error {
	if err == nil {
		return nil
	}
	return s.Event(SSEEventError, map[string]string{
		"error": err.Error(),
	})
}

func (s *SSEWriter) Flush() {
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
