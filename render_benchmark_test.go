package partial

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/donseba/go-partial/exp/templatehelpers"
)

type benchmarkRow struct {
	ID     int
	Name   string
	Price  string
	Status string
}

func BenchmarkRenderLayoutNoCache(b *testing.B) {
	benchmarkRenderReusableLayout(b, false)
}

func BenchmarkRenderLayoutWithCache(b *testing.B) {
	benchmarkRenderReusableLayout(b, true)
}

func BenchmarkRenderLayoutPerRequestNoCache(b *testing.B) {
	benchmarkRenderPerRequestLayout(b, false)
}

func BenchmarkRenderLayoutPerRequestWithCache(b *testing.B) {
	benchmarkRenderPerRequestLayout(b, true)
}

func BenchmarkRenderWithRequestSimpleNoCache(b *testing.B) {
	benchmarkRenderWithRequestSimple(b, false)
}

func BenchmarkRenderWithRequestSimpleWithCache(b *testing.B) {
	benchmarkRenderWithRequestSimple(b, true)
}

func benchmarkRenderWithRequestSimple(b *testing.B, useCache bool) {
	partial := NewID("content", "templates/simple.gohtml").
		SetFileSystem(benchmarkFS()).
		UseTemplateCache(useCache).
		SetDot(map[string]any{
			"Title": "Benchmark",
			"Body":  "A small direct render.",
		})
	request := benchmarkRequest()
	ctx := context.Background()

	if useCache {
		if _, err := partial.RenderWithRequest(ctx, request); err != nil {
			b.Fatalf("prime render: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, err := partial.RenderWithRequest(ctx, request)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("empty render output")
		}
	}
}

func benchmarkRenderReusableLayout(b *testing.B, useCache bool) {
	svc := NewService(&Config{
		FS:               benchmarkFS(),
		UseTemplateCache: useCache,
	})
	svc.SetFunc(templatehelpers.FuncMap())
	content := benchmarkContentPartial()
	wrapper := benchmarkLayoutPartial()
	request := benchmarkRequest()
	ctx := context.Background()

	if useCache {
		if _, err := svc.NewLayout().Set(content).Wrap(wrapper).RenderWithRequest(ctx, request); err != nil {
			b.Fatalf("prime render: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, err := svc.NewLayout().Set(content).Wrap(wrapper).RenderWithRequest(ctx, request)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("empty render output")
		}
	}
}

func benchmarkRenderPerRequestLayout(b *testing.B, useCache bool) {
	svc := NewService(&Config{
		FS:               benchmarkFS(),
		UseTemplateCache: useCache,
	})
	svc.SetFunc(templatehelpers.FuncMap())
	request := benchmarkRequest()
	ctx := context.Background()

	if useCache {
		if _, err := svc.NewLayout().Set(benchmarkContentPartial()).Wrap(benchmarkLayoutPartial()).RenderWithRequest(ctx, request); err != nil {
			b.Fatalf("prime render: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, err := svc.NewLayout().Set(benchmarkContentPartial()).Wrap(benchmarkLayoutPartial()).RenderWithRequest(ctx, request)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("empty render output")
		}
	}
}

func benchmarkContentPartial() *Partial {
	row := NewID("row", "templates/row.gohtml")
	return NewID("content", "templates/content.gohtml").
		SetDot(map[string]any{
			"Title": "Benchmark table",
			"Owner": "Ada",
			"Rows":  benchmarkRows(40),
		}).
		With(row)
}

func benchmarkLayoutPartial() *Partial {
	notice := NewID("notice", "templates/notice.gohtml").
		SetDot(map[string]any{"Message": "Rendered as an OOB child"}).
		SetAlwaysSwapOOB(true)
	return NewID("layout", "templates/layout.gohtml").
		SetDot(map[string]any{"App": "Bench App"}).
		WithOOB(notice)
}

func benchmarkRows(count int) []benchmarkRow {
	rows := make([]benchmarkRow, count)
	for i := range rows {
		rows[i] = benchmarkRow{
			ID:     i + 1,
			Name:   fmt.Sprintf("Product %02d", i+1),
			Price:  fmt.Sprintf("EUR %d.%02d", 10+i, i%100),
			Status: "ready",
		}
	}
	return rows
}

func benchmarkFS() *inMemoryFS {
	return &inMemoryFS{Files: map[string]string{
		"templates/layout.gohtml":  `<!doctype html><html><body><header>{{ .App }}</header>{{ content }}</body></html>`,
		"templates/content.gohtml": `<section><h1>{{ .Title }}</h1><table>{{ range .Rows }}{{ template "row.gohtml" (dict "Row" . "Owner" $.Owner) }}{{ end }}</table></section>`,
		"templates/row.gohtml":     `<tr id="row-{{ .Row.ID }}"><td>{{ .Row.Name }}</td><td>{{ .Row.Price }}</td><td>{{ .Row.Status }}</td><td>{{ .Owner }}</td></tr>`,
		"templates/notice.gohtml":  `<aside id="notice"{{ oobAttr }}>{{ .Message }}</aside>`,
		"templates/simple.gohtml":  `<article><h1>{{ .Title }}</h1><p>{{ .Body }}</p></article>`,
	}}
}

func benchmarkRequest() *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "/benchmark", nil)
	return req
}
