package partial

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-partial/connector"
)

type lazyStats struct {
	Count int
}

func lazyFS() fstest.MapFS {
	return fstest.MapFS{
		"layout.gohtml": {Data: []byte(`<main>{{ content }}</main>`)},
		"stats.gohtml": {Data: []byte(`{{/*
@dot github.com/donseba/go-partial.lazyStats
*/}}<div id="stats">{{ .Count }}</div>`)},
		"inner.gohtml": {Data: []byte(`[{{ .Count }}]`)},
		"outer.gohtml": {Data: []byte(`{{ .Count }}{{ partial runtime "inner.gohtml" }}`)},
	}
}

func TestDotFuncOfIsComputedWhenThePartialRenders(t *testing.T) {
	var calls atomic.Int32

	stats := NewID("stats", "stats.gohtml").SetDotSource(DotFuncOf(func(ctx context.Context) (lazyStats, error) {
		calls.Add(1)
		return lazyStats{Count: 3}, nil
	}))
	other := NewID("other", "inner.gohtml").SetDot(lazyStats{Count: 1})

	root := NewID("root", "layout.gohtml").
		SetFileSystem(lazyFS()).
		SetConnector(connector.NewHTMX(nil)).
		SetContent(other).
		With(stats)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "other")

	if _, err := RenderWithRequest(context.Background(), req, root); err != nil {
		t.Fatal(err)
	}

	if calls.Load() != 0 {
		t.Fatalf("the dot was computed %d times for a response without the partial", calls.Load())
	}

	req.Header.Set("HX-Target", "stats")

	out, err := RenderWithRequest(context.Background(), req, root)
	if err != nil || string(out) != `<div id="stats">3</div>` {
		t.Fatalf("RenderWithRequest() = %q, %v", out, err)
	}

	if calls.Load() != 1 {
		t.Fatalf("the dot was computed %d times, want once", calls.Load())
	}
}

func TestDotFuncOfIsComputedOncePerRenderTree(t *testing.T) {
	var calls atomic.Int32

	root := NewID("outer", "outer.gohtml").
		SetFileSystem(lazyFS()).
		SetDotSource(DotFuncOf(func(ctx context.Context) (lazyStats, error) {
			calls.Add(1)
			return lazyStats{Count: 7}, nil
		}))

	out, err := Render(context.Background(), root)
	if err != nil || string(out) != "7[7]" {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	if calls.Load() != 1 {
		t.Fatalf("the inherited dot was computed %d times, want once", calls.Load())
	}

	if _, err := Render(context.Background(), root); err != nil || calls.Load() != 2 {
		t.Fatalf("a new render should compute the dot again, calls = %d, err = %v", calls.Load(), err)
	}
}

func TestDotFuncOfErrorFailsTheRender(t *testing.T) {
	root := NewID("stats", "stats.gohtml").
		SetFileSystem(lazyFS()).
		SetDotSource(DotFuncOf(func(ctx context.Context) (lazyStats, error) {
			return lazyStats{}, errors.New("database down")
		}))

	if _, err := Render(context.Background(), root); err == nil || !strings.Contains(err.Error(), "database down") {
		t.Fatalf("Render() error = %v", err)
	}
}

func TestSetDotReplacesALazyDot(t *testing.T) {
	root := NewID("stats", "stats.gohtml").
		SetFileSystem(lazyFS()).
		SetDotSource(DotFuncOf(func(ctx context.Context) (lazyStats, error) {
			return lazyStats{}, errors.New("not called")
		})).
		SetDot(lazyStats{Count: 2})

	if out, err := Render(context.Background(), root); err != nil || string(out) != `<div id="stats">2</div>` {
		t.Fatalf("Render() = %q, %v", out, err)
	}
}

func TestCheckContractsWithALazyDot(t *testing.T) {
	called := false

	typed := NewID("stats", "stats.gohtml").SetFileSystem(lazyFS())
	typed.SetDotSource(DotFuncOf(func(ctx context.Context) (lazyStats, error) {
		called = true
		return lazyStats{}, nil
	}))

	if err := typed.CheckContracts(); err != nil {
		t.Fatalf("CheckContracts() error = %v", err)
	}

	wrong := NewID("stats", "stats.gohtml").SetFileSystem(lazyFS())
	wrong.SetDotSource(DotFuncOf(func(ctx context.Context) (string, error) {
		called = true
		return "", nil
	}))

	if err := wrong.CheckContracts(); err == nil || !strings.Contains(err.Error(), "@dot expects") {
		t.Fatalf("CheckContracts() error = %v, want a @dot mismatch", err)
	}

	untyped := NewID("stats", "stats.gohtml").SetFileSystem(lazyFS())
	untyped.SetDotSource(DotFuncOf(func(ctx context.Context) (any, error) {
		called = true
		return "", nil
	}))

	if err := untyped.CheckContracts(); err != nil {
		t.Fatalf("CheckContracts() error = %v; a lazy dot of an interface type is not checked", err)
	}

	if called {
		t.Fatal("CheckContracts computed a lazy dot")
	}
}
