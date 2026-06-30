package partial

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/donseba/go-partial/connector"
)

type requestContextTestKey struct{}

func TestGetBasePathSimple(t *testing.T) {
	p := New()
	p.SetBasePath("/foo")
	if got := p.GetBasePath(); got != "/foo" {
		t.Errorf("expected /foo, got %q", got)
	}
}

func TestGetBasePathParentFallback(t *testing.T) {
	parent := New()
	parent.SetBasePath("/parent")
	child := NewID("child")
	parent.With(child)
	if got := child.GetBasePath(); got != "/parent" {
		t.Errorf("expected /parent, got %q", got)
	}
}

func TestGetBasePathParentChain(t *testing.T) {
	grandparent := New()
	grandparent.SetBasePath("/grand")
	parent := NewID("parent")
	child := NewID("child")
	grandparent.With(parent)
	parent.With(child)
	if got := child.GetBasePath(); got != "/grand" {
		t.Errorf("expected /grand, got %q", got)
	}
}

func TestGetBasePathOverride(t *testing.T) {
	parent := New()
	parent.SetBasePath("/parent")
	child := NewID("child")
	parent.With(child)
	child.SetBasePath("/child")
	if got := child.GetBasePath(); got != "/child" {
		t.Errorf("expected /child, got %q", got)
	}
}

func TestGetBasePathEmpty(t *testing.T) {
	p := New()
	if got := p.GetBasePath(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTemplateBasePathUsesChildOverride(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"child.gohtml": `{{ basePath }}`,
		},
	}
	parent := NewID("parent")
	child := NewID("child", "child.gohtml").SetFileSystem(fsys)
	parent.SetBasePath("/parent").With(child)
	child.SetBasePath("/child")

	out, err := child.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "/child" {
		t.Fatalf("basePath helper = %q, want %q", got, "/child")
	}
}

func TestChildFileSystemOverridesParentFileSystem(t *testing.T) {
	parentFS := &inMemoryFS{
		Files: map[string]string{
			"shared.gohtml": `parent`,
		},
	}
	childFS := &inMemoryFS{
		Files: map[string]string{
			"shared.gohtml": `child`,
		},
	}
	parent := NewID("parent").SetFileSystem(parentFS)
	child := NewID("child", "shared.gohtml").SetFileSystem(childFS)
	parent.With(child)

	out, err := child.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "child" {
		t.Fatalf("child rendered %q, want child filesystem content", got)
	}
}

func TestChildResponseHeadersDoNotLeakToParent(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"parent.gohtml": `parent`,
			"child.gohtml":  `child`,
		},
	}
	parent := NewID("parent", "parent.gohtml").SetFileSystem(fsys)
	child := NewID("child", "child.gohtml").SetResponseHeaders(map[string]string{
		"X-Child": "true",
	})
	parent.With(child)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := parent.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("X-Child"); got != "" {
		t.Fatalf("parent response header X-Child = %q, want empty", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Target", "child")
	if err := parent.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("X-Child"); got != "true" {
		t.Fatalf("child response header X-Child = %q, want true", got)
	}
}

func TestLayoutWriteWithRequestWritesContentWithoutWrapper(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"content.gohtml": `content`,
		},
	}
	layout := NewService(&Config{FS: fsys}).NewLayout().Set(NewID("content", "content.gohtml"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := layout.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "content" {
		t.Fatalf("body = %q, want content", got)
	}
}

func TestConcurrentPartialRendersDoNotBleedRequestData(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"page.gohtml": `{{ (request).URL.Query.Get "value" }}`,
		},
	}
	p := NewID("page", "page.gohtml").SetFileSystem(fsys)

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+want, nil)
			out, err := p.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != want {
				errs <- "render " + want + " got " + got
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestConcurrentLayoutRendersDoNotBleedRequestData(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"layout.gohtml":  `<main>{{ content }}</main>`,
			"content.gohtml": `{{ (request).URL.Query.Get "value" }}`,
		},
	}
	layout := NewService(&Config{FS: fsys}).NewLayout().
		Set(NewID("content", "content.gohtml")).
		Wrap(NewID("layout", "layout.gohtml"))

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+want, nil)
			out, err := layout.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != "<main>"+want+"</main>" {
				errs <- "render " + want + " got " + got
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestConcurrentCachedLayoutRendersDoNotBleedRequestData(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"layout.gohtml":  `<main>{{ content }}</main>`,
			"content.gohtml": `{{ requestValue }}`,
		},
	}
	layout := NewService(&Config{FS: fsys, UseTemplateCache: true}).NewLayout().
		SetFunc(template.FuncMap{
			"requestValue": func(ctx *RenderContext) string {
				if ctx == nil || ctx.Request == nil || ctx.Request.URL == nil {
					return ""
				}
				return ctx.Request.URL.Query().Get("value")
			},
		}).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				ctx.SetFunc("requestValue", func() string {
					return ctx.Request.URL.Query().Get("value")
				})
				return ctx, nil
			},
		}).
		Set(NewID("content", "content.gohtml")).
		Wrap(NewID("layout", "layout.gohtml"))

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+want, nil)
			out, err := layout.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != "<main>"+want+"</main>" {
				errs <- "cached render " + want + " got " + got
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestConcurrentRendererValuesAreIsolated(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"page.gohtml": `{{ renderValue }}`,
		},
	}
	p := NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				value := ctx.Request.URL.Query().Get("value")
				ctx.Values.Set(requestContextTestKey{}, value)
				ctx.SetFunc("renderValue", func() string {
					got, _ := ctx.Values.Get(requestContextTestKey{}).(string)
					return got
				})
				return ctx, nil
			},
		})

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+want, nil)
			out, err := p.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != want {
				errs <- "RenderStage " + want + " got " + got
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestConcurrentTargetRendersDoNotBleedOOBState(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"table.gohtml": `<section>table</section>`,
			"row.gohtml":   `<div>{{ (request).URL.Query.Get "value" }}</div>`,
			"toast.gohtml": `<aside>{{ if oob }}oob{{ else }}inline{{ end }}:{{ (request).URL.Query.Get "value" }}</aside>`,
		},
	}
	table := NewID("table", "table.gohtml").
		SetFileSystem(fsys).
		With(NewID("row", "row.gohtml")).
		WithOOB(NewID("toast", "toast.gohtml"))

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+want, nil)
			req.Header.Set(connector.HeaderTarget.String(), "row")
			out, err := table.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			expected := "<div>" + want + "</div><aside>oob:" + want + "</aside>"
			if got := string(out); got != expected {
				errs <- "target " + want + " got " + got
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestConnectorsAreSafeForConcurrentReads(t *testing.T) {
	connectors := []connector.Connector{
		connector.NewPartial(&connector.Config{UseURLQuery: true}),
		connector.NewHTMX(&connector.Config{UseURLQuery: true}),
		connector.NewTurbo(&connector.Config{UseURLQuery: true}),
		connector.NewUnpoly(&connector.Config{UseURLQuery: true}),
	}

	const reads = 64
	var wg sync.WaitGroup
	errs := make(chan string, len(connectors)*reads)
	for _, conn := range connectors {
		conn := conn
		for i := range reads {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				want := strconv.Itoa(i)
				req := httptest.NewRequest(http.MethodGet, "/?target="+want+"&select=s"+want+"&action=a"+want, nil)
				req.Header.Set(conn.GetTargetHeader(), want)
				req.Header.Set(conn.GetSelectHeader(), "s"+want)
				req.Header.Set(conn.GetActionHeader(), "a"+want)
				if got := conn.GetTargetValue(req); got != want {
					errs <- fmt.Sprintf("%T target got %q want %q", conn, got, want)
				}
				if got := conn.GetSelectValue(req); got != "s"+want {
					errs <- fmt.Sprintf("%T select got %q want %q", conn, got, "s"+want)
				}
				if got := conn.GetActionValue(req); got != "a"+want {
					errs <- fmt.Sprintf("%T action got %q want %q", conn, got, "a"+want)
				}
				attrs := conn.InteractionAttrs(connector.Interaction{
					Kind:    connector.InteractionAsync,
					ID:      "item-" + want,
					URL:     "/items/" + want,
					Options: map[string]string{"from": "body"},
				})
				if len(attrs) == 0 {
					errs <- fmt.Sprintf("%T returned no attrs", conn)
				}
				headers := conn.ResponseHeaders(connector.Response{Retarget: "#item-" + want})
				if conn.GetTargetHeader() != connector.TurboHeaderTarget.String() && len(headers) == 0 {
					errs <- fmt.Sprintf("%T returned no response headers", conn)
				}
			}(i)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestRenderWithRequestUsesRequestContextWhenContextIsNil(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"page.gohtml": `{{ requestContextValue }}`,
		},
	}
	p := NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				ctx.SetFunc("requestContextValue", func() string {
					return fmt.Sprint(ctx.Context.Value(requestContextTestKey{}))
				})
				return ctx, nil
			},
		})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestContextTestKey{}, "from-request"))

	//lint:ignore SA1012 this verifies RenderWithRequest falls back to req.Context when ctx is nil.
	out, err := p.RenderWithRequest(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "from-request" {
		t.Fatalf("context value = %q, want from-request", got)
	}
}

func TestRenderNilContextProvidesDefaultContext(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"page.gohtml": `{{ contextReady }}`,
		},
	}
	p := NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				ctx.SetFunc("contextReady", func() string {
					if ctx.Context == nil {
						return "missing"
					}
					return "ready"
				})
				return ctx, nil
			},
		})

	//lint:ignore SA1012 this verifies Render supplies a default context when ctx is nil.
	out, err := p.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "ready" {
		t.Fatalf("contextReady = %q, want ready", got)
	}
}

func TestConcurrentServiceCacheLayoutOOBAndTargetStress(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"layout.gohtml":  `<html>{{ content }}</html>`,
			"content.gohtml": `<main>{{ (request).URL.Query.Get "value" }}</main>`,
			"row.gohtml":     `<tr>{{ (request).URL.Query.Get "value" }}:{{ renderMarker }}</tr>`,
			"toast.gohtml":   `<aside{{ oobAttr }}>{{ (request).URL.Query.Get "value" }}</aside>`,
		},
	}
	service := NewService(&Config{
		FS:               fsys,
		UseTemplateCache: true,
		Connector:        connector.NewPartial(nil),
	})
	service.SetFunc(template.FuncMap{
		"renderMarker": func() string { return "" },
	})
	service.Use(RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			marker := ctx.Request.URL.Query().Get("value")
			ctx.Values.Set(requestContextTestKey{}, marker)
			ctx.SetFunc("renderMarker", func() string {
				value, _ := ctx.Values.Get(requestContextTestKey{}).(string)
				return value
			})
			return ctx, nil
		},
	})

	content := NewID("content", "content.gohtml").With(NewID("row", "row.gohtml"))
	wrapper := NewID("layout", "layout.gohtml").WithOOB(NewID("toast", "toast.gohtml"))
	layout := service.NewLayout().Set(content).Wrap(wrapper)

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+value, nil)
			if i%2 == 0 {
				req.Header.Set(connector.HeaderTarget.String(), "row")
			}
			out, err := layout.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			want := `<html><main>` + value + `</main></html>`
			if i%2 == 0 {
				want = `<tr>` + value + `:` + value + `</tr><aside hx-swap-oob="true">` + value + `</aside>`
			}
			if got := string(out); got != want {
				errs <- "stress " + value + " got " + got + " want " + want
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
