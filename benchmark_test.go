package partial

import (
	"context"
	"fmt"
	"net/http"
	"testing"
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

func benchmarkRenderReusableLayout(b *testing.B, useCache bool) {
	resetBenchmarkTemplateCache(b)

	svc := NewService(&Config{
		FS:               benchmarkFS(),
		UseTemplateCache: useCache,
	})
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
	resetBenchmarkTemplateCache(b)

	svc := NewService(&Config{
		FS:               benchmarkFS(),
		UseTemplateCache: useCache,
	})
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
		SetData(map[string]any{
			"Title": "Benchmark table",
			"Owner": "Ada",
			"Rows":  benchmarkRows(40),
		}).
		With(row)
}

func benchmarkLayoutPartial() *Partial {
	notice := NewID("notice", "templates/notice.gohtml").
		SetData(map[string]any{"Message": "Rendered as an OOB child"}).
		SetAlwaysSwapOOB(true)
	return NewID("layout", "templates/layout.gohtml").WithOOB(notice)
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
		"templates/layout.gohtml":  `<!doctype html><html><body><header>{{ .Service.App }}</header>{{ child "content" }}{{ child "notice" }}</body></html>`,
		"templates/content.gohtml": `<section><h1>{{ .Data.Title }}</h1><table>{{ range .Data.Rows }}{{ partial "row" "Row" . "Owner" $.Data.Owner }}{{ end }}</table></section>`,
		"templates/row.gohtml":     `<tr id="row-{{ scoped.Row.ID }}"><td>{{ scoped.Row.Name }}</td><td>{{ scoped.Row.Price }}</td><td>{{ scoped.Row.Status }}</td><td>{{ scoped.Owner }}</td></tr>`,
		"templates/notice.gohtml":  `<aside id="notice"{{ oobAttr }}>{{ .Data.Message }}</aside>`,
	}}
}

func benchmarkRequest() *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "/benchmark", nil)
	return req
}

func resetBenchmarkTemplateCache(b *testing.B) {
	b.Helper()
	templateCache.Clear()
	mutexCache.Clear()
}
