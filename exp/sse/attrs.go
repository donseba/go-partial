package sse

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/donseba/go-partial/internal/templateutil"
)

// ClientStream is an element whose stream the browser client of this
// package (ClientScript) opens, as attributes; see Attrs. Templates write
// the same attributes with {{ sseAttrs }}.
//
//	stream := sse.ClientStream{
//		URL:      "/presence?page=/orders",
//		ClientID: "tab",
//		Hidden:   30 * time.Second,
//		Beat:     15 * time.Second,
//		Beacon:   "/presence",
//	}
//	attrs := stream.Attrs()
type ClientStream struct {
	// URL is the address of the stream: data-partial-stream. Without it,
	// Attrs writes nothing.
	URL string
	// ClientID names the parameter of the stream URL that the client
	// adds the id of the browser tab to, such as "tab":
	// data-partial-stream-client-id.
	ClientID string
	// Hidden closes the stream once the tab has been hidden that long,
	// and opens it again when the tab shows:
	// data-partial-stream-hidden.
	Hidden time.Duration
	// Beat reports the stream to Beacon at this interval, while the
	// element is in the page, also while the tab is hidden and the
	// stream closed: data-partial-stream-beat.
	Beat time.Duration
	// Beacon is the address the client reports the stream to, with its
	// beats and when it is hidden, removed or the page is left, which
	// ReadBeacon reads: data-partial-stream-beacon.
	Beacon string
	// Credentials sends cookies with a stream of another origin:
	// data-partial-credentials.
	Credentials bool
}

// FuncMap returns the template helpers of the browser client:
// {{ sseAttrs }} (see Attrs).
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"sseAttrs": Attrs,
	}
}

// Attrs returns the attributes of an element of the template's own whose
// stream the browser client of this package (ClientScript) opens, the
// stream URL followed by key/value pairs of ClientStream's settings:
// "clientId", "hidden", "beat" (durations such as "30s" or a
// time.Duration), "beacon" and "credentials":
//
//	<div{{ sseAttrs .Stream "clientId" "tab" "hidden" "30s" "beat" "15s" "beacon" "/presence" }}>
//
// An empty URL writes nothing, so a page without a stream needs no
// condition; invalid settings write a data-partial-error attribute with
// the error. Register it with FuncMap as "sseAttrs".
//
// go-doc:sig func(url string, pairs ...any) html/template.HTMLAttr
func Attrs(url string, pairs ...any) template.HTMLAttr {
	stream, err := clientStream(url, pairs)
	if err != nil {
		return template.HTMLAttr(` data-partial-error="` + template.HTMLEscapeString(err.Error()) + `"`)
	}

	return stream.Attrs()
}

// clientStream reads the settings of Attrs.
func clientStream(url string, pairs []any) (ClientStream, error) {
	stream := ClientStream{URL: url}

	if len(pairs)%2 != 0 {
		return stream, fmt.Errorf("sseAttrs expects key/value pairs")
	}

	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return stream, fmt.Errorf("sseAttrs: key must be a string, got %T", pairs[i])
		}

		value := pairs[i+1]

		var err error

		switch key {
		case "clientId":
			stream.ClientID = fmt.Sprint(value)
		case "beacon":
			stream.Beacon = fmt.Sprint(value)
		case "hidden":
			stream.Hidden, err = durationOf(key, value)
		case "beat":
			stream.Beat, err = durationOf(key, value)
		case "credentials":
			stream.Credentials, err = flagOf(key, value)
		default:
			err = fmt.Errorf("sseAttrs: unknown setting %q", key)
		}

		if err != nil {
			return stream, err
		}
	}

	return stream, nil
}

func durationOf(key string, value any) (time.Duration, error) {
	switch v := value.(type) {
	case time.Duration:
		return v, nil
	case string:
		duration, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("sseAttrs: %s must be a duration such as 30s, got %q", key, v)
		}

		return duration, nil
	default:
		return 0, fmt.Errorf("sseAttrs: %s must be a duration such as 30s, got %T", key, value)
	}
}

func flagOf(key string, value any) (bool, error) {
	switch strings.ToLower(fmt.Sprint(value)) {
	case "true", "on":
		return true, nil
	case "false", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("sseAttrs: %s must be true or false, got %v", key, value)
	}
}

// Attrs returns the attributes of the stream's element, each after a
// space, or nothing without a URL.
func (s ClientStream) Attrs() template.HTMLAttr {
	if s.URL == "" {
		return ""
	}

	var b strings.Builder

	templateutil.WriteAttr(&b, "data-partial-stream", s.URL)

	if s.ClientID != "" {
		templateutil.WriteAttr(&b, "data-partial-stream-client-id", s.ClientID)
	}

	if s.Hidden > 0 {
		templateutil.WriteAttr(&b, "data-partial-stream-hidden", clientDuration(s.Hidden))
	}

	if s.Beat > 0 {
		templateutil.WriteAttr(&b, "data-partial-stream-beat", clientDuration(s.Beat))
	}

	if s.Beacon != "" {
		templateutil.WriteAttr(&b, "data-partial-stream-beacon", s.Beacon)
	}

	if s.Credentials {
		b.WriteString(" data-partial-credentials")
	}

	return template.HTMLAttr(b.String())
}

// clientDuration writes d as the client reads it: whole seconds as "30s",
// anything else in milliseconds.
func clientDuration(d time.Duration) string {
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", d/time.Second)
	}

	return fmt.Sprintf("%dms", d/time.Millisecond)
}
