package sse

import (
	"context"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	partial "github.com/donseba/go-partial"
)

func TestClientStreamAttrs(t *testing.T) {
	tests := []struct {
		name   string
		stream ClientStream
		want   template.HTMLAttr
	}{
		{"no url", ClientStream{ClientID: "tab"}, ""},
		{"url alone", ClientStream{URL: "/events"}, ` data-partial-stream="/events"`},
		{
			"every setting",
			ClientStream{
				URL:         "/presence?page=/a&b",
				ClientID:    "tab",
				Hidden:      30 * time.Second,
				Beat:        1500 * time.Millisecond,
				Beacon:      "/presence",
				Credentials: true,
			},
			` data-partial-stream="/presence?page=/a&amp;b" data-partial-stream-client-id="tab" data-partial-stream-hidden="30s"` +
				` data-partial-stream-beat="1500ms" data-partial-stream-beacon="/presence" data-partial-credentials`,
		},
	}

	for _, test := range tests {
		if got := test.stream.Attrs(); got != test.want {
			t.Errorf("%s: Attrs() =\n%s\nwant\n%s", test.name, got, test.want)
		}
	}
}

func TestSSEAttrsInATemplate(t *testing.T) {
	files := fstest.MapFS{
		"header.gohtml": {
			Data: []byte(`<div{{ sseAttrs .Stream "clientId" "tab" "hidden" "30s" "beat" "15s" "beacon" "/presence" }}></div>`),
		},
	}

	render := func(stream string) string {
		header := partial.NewID("header", "header.gohtml").
			SetFileSystem(files).
			SetFunc(FuncMap()).
			SetDot(map[string]string{"Stream": stream})

		out, err := partial.Render(context.Background(), header)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		return string(out)
	}

	want := `<div data-partial-stream="/presence?page=%2Forders" data-partial-stream-client-id="tab" data-partial-stream-hidden="30s"` +
		` data-partial-stream-beat="15s" data-partial-stream-beacon="/presence"></div>`
	if got := render("/presence?page=%2Forders"); got != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", got, want)
	}

	if got := render(""); got != `<div></div>` {
		t.Fatalf("Render() without a stream = %s", got)
	}
}

func TestSSEAttrsReportsInvalidSettings(t *testing.T) {
	for _, pairs := range [][]any{
		{"hidden"},
		{"hidden", "soon"},
		{"beat", 15},
		{"credentials", "maybe"},
		{"colour", "red"},
		{3, "x"},
	} {
		if got := Attrs("/events", pairs...); !strings.Contains(string(got), "data-partial-error") {
			t.Errorf("Attrs(%v) = %s, want an error", pairs, got)
		}
	}

	got := Attrs("/events", "beat", 2*time.Minute, "credentials", true)
	if got != ` data-partial-stream="/events" data-partial-stream-beat="120s" data-partial-credentials` {
		t.Fatalf("Attrs() with Go values = %s", got)
	}
}
