package metrics

import (
	"context"
	"errors"
	"html/template"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func TestRendererRecordsRenderMetrics(t *testing.T) {
	var records []Record
	now := tickingClock(
		time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
		25*time.Millisecond,
	)

	p := partial.New("page.gohtml").
		ID("metrics-page").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`Hello {{ .Name }}`)},
		}).
		SetDot(map[string]string{"Name": "Ada"}).
		Use(Renderer(SinkFunc(func(record Record) {
			records = append(records, record)
		}), WithClock(now), WithTag("area", "showcase")))
	WithPartialTag(p, "main panel")

	req := httptest.NewRequest("GET", "/metrics", nil)
	out, err := p.RenderWithRequest(WithRequestID(context.Background(), "req-test"), req)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if out != "Hello Ada" {
		t.Fatalf("Render() = %q", out)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	record := records[0]
	if record.Kind != partial.RenderKindPartial {
		t.Fatalf("Kind = %q, want %q", record.Kind, partial.RenderKindPartial)
	}
	if record.PartialID != "metrics-page" {
		t.Fatalf("PartialID = %q", record.PartialID)
	}
	if record.PartialTag != "main panel" {
		t.Fatalf("PartialTag = %q", record.PartialTag)
	}
	if record.RequestID != "req-test" {
		t.Fatalf("RequestID = %q", record.RequestID)
	}
	if len(record.Templates) != 1 || record.Templates[0] != "page.gohtml" {
		t.Fatalf("Templates = %#v", record.Templates)
	}
	if record.Method != "GET" || record.Path != "/metrics" {
		t.Fatalf("request = %s %s", record.Method, record.Path)
	}
	if record.Size != len("Hello Ada") {
		t.Fatalf("Size = %d", record.Size)
	}
	if !record.Rendered {
		t.Fatal("Rendered = false, want true")
	}
	if record.Duration != 25*time.Millisecond {
		t.Fatalf("Duration = %s", record.Duration)
	}
	if record.Tags["area"] != "showcase" {
		t.Fatalf("Tags = %#v", record.Tags)
	}
	if record.Error != nil {
		t.Fatalf("Error = %v", record.Error)
	}
}

func TestRendererUsesRequestIDHeaderFallback(t *testing.T) {
	var record Record
	p := partial.New("page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`ok`)},
		}).
		Use(Renderer(SinkFunc(func(next Record) {
			record = next
		})))

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set(HeaderRequestID, "req-header")
	if _, err := p.RenderWithRequest(context.Background(), req); err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if record.RequestID != "req-header" {
		t.Fatalf("RequestID = %q", record.RequestID)
	}
}

func TestRendererMarksOOBPartials(t *testing.T) {
	var records []Record
	svc := partial.NewService(&partial.Config{
		Connector: connector.NewHTMX(nil),
		FS: fstest.MapFS{
			"layout.gohtml":  &fstest.MapFile{Data: []byte(`{{ content }}`)},
			"content.gohtml": &fstest.MapFile{Data: []byte(`<main>content</main>`)},
			"header.gohtml":  &fstest.MapFile{Data: []byte(`<aside{{ oobAttr }}>header</aside>`)},
		},
		Renderers: []partial.Renderer{
			Renderer(SinkFunc(func(record Record) {
				records = append(records, record)
			})),
		},
	})

	content := WithPartialTag(partial.NewID("content", "content.gohtml"), "main")
	wrapper := WithPartialTag(partial.NewID("layout", "layout.gohtml"), "shell").
		WithOOB(WithPartialTag(partial.NewID("header", "header.gohtml").SetAlwaysSwapOOB(true), "sidebar"))
	layout := svc.NewLayout().Set(content).Wrap(wrapper)

	req := httptest.NewRequest("GET", "/rows", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")
	recorder := httptest.NewRecorder()
	if err := layout.WriteWithRequest(WithRequestID(context.Background(), "req-oob"), recorder, req); err != nil {
		t.Fatalf("WriteWithRequest() error = %v", err)
	}

	seenContent := false
	seenHeader := false
	for _, record := range records {
		if record.RequestID != "req-oob" {
			t.Fatalf("RequestID = %q", record.RequestID)
		}
		switch record.PartialID {
		case "content":
			seenContent = true
			if record.PartialTag != "main" {
				t.Fatalf("content PartialTag = %q", record.PartialTag)
			}
			if record.OOB {
				t.Fatal("content OOB = true, want false")
			}
		case "header":
			seenHeader = true
			if record.PartialTag != "sidebar" {
				t.Fatalf("header PartialTag = %q", record.PartialTag)
			}
			if !record.OOB {
				t.Fatal("header OOB = false, want true")
			}
		}
	}
	if !seenContent || !seenHeader {
		t.Fatalf("records = %#v, want content and header", records)
	}
}

func TestRendererRecordsRenderErrors(t *testing.T) {
	renderErr := errors.New("boom")
	var records []Record
	renderer := Renderer(SinkFunc(func(record Record) {
		records = append(records, record)
	}))
	ctx := &partial.RenderContext{
		Kind:   partial.RenderKindPartial,
		Values: make(partial.RenderValues),
	}

	ctx, err := renderer.Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	out, err := renderer.Render(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return template.HTML("partial output"), renderErr
	})
	out, err = renderer.Finalize(ctx, out, err)
	if !errors.Is(err, renderErr) {
		t.Fatalf("Finalize() error = %v, want %v", err, renderErr)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Size != len("partial output") {
		t.Fatalf("Size = %d", record.Size)
	}
	if !record.Rendered {
		t.Fatal("Rendered = false, want true")
	}
	if !errors.Is(record.Error, renderErr) {
		t.Fatalf("record.Error = %v, want %v", record.Error, renderErr)
	}
}

func tickingClock(start time.Time, step time.Duration) func() time.Time {
	current := start.Add(-step)
	return func() time.Time {
		current = current.Add(step)
		return current
	}
}
