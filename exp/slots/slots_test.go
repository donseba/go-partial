package slots

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/metrics"
)

func TestSlotRendersChildPartial(t *testing.T) {
	parent := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml":    &fstest.MapFile{Data: []byte(`<main>{{ slot "toolbar" }}</main>`)},
			"toolbar.gohtml": &fstest.MapFile{Data: []byte(`<nav>{{ .Label }}</nav>`)},
		}).
		SetDot(map[string]string{"Title": "Page"}).
		Use(Renderer())
	child := partial.NewID("toolbar", "toolbar.gohtml").
		SetDot(map[string]string{"Label": "Actions"})
	Set(parent, "toolbar", child)

	out, err := parent.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := string(out); !strings.Contains(got, `<nav>Actions</nav>`) {
		t.Fatalf("Render() = %q", got)
	}
}

func TestHasSlot(t *testing.T) {
	parent := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml":    &fstest.MapFile{Data: []byte(`{{ if hasSlot "toolbar" }}yes{{ end }}{{ if hasSlot "missing" }}no{{ end }}`)},
			"toolbar.gohtml": &fstest.MapFile{Data: []byte(`toolbar`)},
		}).
		Use(Renderer())
	Set(parent, "toolbar", partial.NewID("toolbar", "toolbar.gohtml"))

	out, err := parent.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := string(out); got != "yes" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestMissingSlotRendersEmpty(t *testing.T) {
	parent := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`before{{ slot "missing" }}after`)},
		}).
		Use(Renderer())

	out, err := parent.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := string(out); got != "beforeafter" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestSlotCreatesChildRenderMetrics(t *testing.T) {
	var records []metrics.Record
	parent := metrics.WithPartialLabel(partial.NewID("page", "page.gohtml"), "shell").
		SetFileSystem(fstest.MapFS{
			"page.gohtml":    &fstest.MapFile{Data: []byte(`<main>{{ slot "toolbar" }}</main>`)},
			"toolbar.gohtml": &fstest.MapFile{Data: []byte(`<nav>Actions</nav>`)},
		}).
		Use(Renderer(), metrics.Renderer(metrics.SinkFunc(func(record metrics.Record) {
			records = append(records, record)
		}), metrics.WithSlotName(Name)))
	child := metrics.WithPartialLabel(partial.NewID("toolbar", "toolbar.gohtml"), "actions")
	Set(parent, "toolbar", child)

	if _, err := parent.Render(metrics.WithRequestID(context.Background(), "req-slot")); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	seenParent := false
	childRecords := 0
	for _, record := range records {
		if record.RequestID != "req-slot" {
			t.Fatalf("RequestID = %q", record.RequestID)
		}
		switch record.PartialID {
		case "page":
			seenParent = true
			if record.ParentID != "" {
				t.Fatalf("page ParentID = %q", record.ParentID)
			}
		case "toolbar":
			childRecords++
			if record.ParentID != "page" {
				t.Fatalf("toolbar ParentID = %q", record.ParentID)
			}
			if record.SlotName != "toolbar" {
				t.Fatalf("toolbar SlotName = %q", record.SlotName)
			}
			if record.PartialLabel != "actions" {
				t.Fatalf("toolbar PartialLabel = %q", record.PartialLabel)
			}
		}
	}
	if !seenParent || childRecords != 1 {
		t.Fatalf("records = %#v, want parent and child", records)
	}
}
