package slots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
		Use(Stage())
	child := partial.NewID("toolbar", "toolbar.gohtml").
		SetDot(map[string]string{"Label": "Actions"})
	Set(parent, "toolbar", child)

	out, err := partial.Render(context.Background(), parent)
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
		Use(Stage())
	Set(parent, "toolbar", partial.NewID("toolbar", "toolbar.gohtml"))

	out, err := partial.Render(context.Background(), parent)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := string(out); got != "yes" {
		t.Fatalf("Render() = %q, want %q", got, "yes")
	}
}

func TestMissingSlotRendersEmpty(t *testing.T) {
	parent := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`before{{ slot "missing" }}after`)},
		}).
		Use(Stage())

	out, err := partial.Render(context.Background(), parent)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := string(out); got != "beforeafter" {
		t.Fatalf("Render() = %q, want %q", got, "beforeafter")
	}
}

func TestSlotCreatesChildRenderMetrics(t *testing.T) {
	var records []metrics.Record
	parent := metrics.WithPartialLabel(partial.NewID("page", "page.gohtml"), "shell").
		SetFileSystem(fstest.MapFS{
			"page.gohtml":    &fstest.MapFile{Data: []byte(`<main>{{ slot "toolbar" }}</main>`)},
			"toolbar.gohtml": &fstest.MapFile{Data: []byte(`<nav>Actions</nav>`)},
		}).
		Use(Stage(), metrics.Stage(metrics.SinkFunc(func(record metrics.Record) {
			records = append(records, record)
		}), metrics.WithSlotName(Name)))
	child := metrics.WithPartialLabel(partial.NewID("toolbar", "toolbar.gohtml"), "actions")
	Set(parent, "toolbar", child)

	if _, err := partial.Render(metrics.WithRequestID(context.Background(), "req-slot"), parent); err != nil {
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

func TestSlotRendersConcurrently(t *testing.T) {
	parent := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml":    &fstest.MapFile{Data: []byte(`<main>{{ slot "toolbar" }}</main>`)},
			"toolbar.gohtml": &fstest.MapFile{Data: []byte(`<nav>{{ (request).URL.Query.Get "value" }}</nav>`)}},
		).
		Use(Stage())
	Set(parent, "toolbar", partial.NewID("toolbar", "toolbar.gohtml"))

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+value, nil)
			out, err := partial.RenderWithRequest(req.Context(), req, parent)
			if err != nil {
				errs <- err.Error()
				return
			}
			want := `<main><nav>` + value + `</nav></main>`
			if got := string(out); got != want {
				errs <- "slot " + value + " got " + got + " want " + want
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
