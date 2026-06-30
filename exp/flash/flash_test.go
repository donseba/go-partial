package flash

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestRendererRendersDefaultTemplate(t *testing.T) {
	ctx := Add(context.Background(), Success("Saved"), Warn("Slow"))
	content := partial.NewID("content", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `{{ flash }}`,
	}))
	content.SetFunc(FuncMap())
	content.Use(Stage())

	out, err := content.Render(ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := string(out)
	if !strings.Contains(html, "Saved") || !strings.Contains(html, "Slow") {
		t.Fatalf("expected flash messages in output: %s", html)
	}
	if !strings.Contains(html, `data-flash-level="success"`) || !strings.Contains(html, `data-flash-level="warn"`) {
		t.Fatalf("expected default level markers in output: %s", html)
	}
}

func TestRendererUsesOverrideTemplate(t *testing.T) {
	ctx := Add(context.Background(), Error("Nope"))
	fsys := testFS(map[string]string{
		"page.gohtml":  `{{ flash }}`,
		"flash.gohtml": `<aside>{{ range .Messages }}{{ .Level }}:{{ .Text }}{{ end }}</aside>`,
	})
	content := partial.NewID("content", "page.gohtml").SetFileSystem(fsys)
	content.SetFunc(FuncMap())
	content.Use(Stage(WithTemplate("flash.gohtml")))

	out, err := content.Render(ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := string(out); !strings.Contains(got, "<aside>error:Nope</aside>") {
		t.Fatalf("expected override template, got %s", got)
	}
}

func TestRendererRendersDefaultTarget(t *testing.T) {
	content := partial.NewID("content", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `{{ flashTarget }}`,
	}))
	content.SetFunc(FuncMap())
	content.Use(Stage())

	out, err := content.Render(context.Background())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := string(out)
	if !strings.Contains(html, `id="flash-messages"`) || !strings.Contains(html, `partial-flash-target`) {
		t.Fatalf("expected default flash target, got %s", html)
	}
}

func TestRendererUsesOverrideTarget(t *testing.T) {
	fsys := testFS(map[string]string{
		"page.gohtml":   `{{ flashTarget }}`,
		"target.gohtml": `<aside id="{{ .TargetID }}" class="toast-stack"></aside>`,
	})
	content := partial.NewID("content", "page.gohtml").SetFileSystem(fsys)
	content.SetFunc(FuncMap())
	content.Use(Stage(WithTargetID("notices"), WithTargetTemplate("target.gohtml")))

	out, err := content.Render(context.Background())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := string(out); !strings.Contains(got, `<aside id="notices" class="toast-stack"></aside>`) {
		t.Fatalf("expected override target template, got %s", got)
	}
}

func TestTargetIDIsNormalized(t *testing.T) {
	fsys := testFS(map[string]string{
		"page.gohtml":   `{{ flashTarget }}`,
		"target.gohtml": `<aside id="{{ .TargetID }}"></aside>`,
	})
	content := partial.NewID("content", "page.gohtml").SetFileSystem(fsys)
	content.SetFunc(FuncMap())
	content.Use(Stage(WithTargetID("# 42 bad:id<script>"), WithTargetTemplate("target.gohtml")))

	out, err := content.Render(context.Background())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := string(out); !strings.Contains(got, `id="flash-42-bad-id-script"`) {
		t.Fatalf("expected normalized target id, got %s", got)
	}
}

func TestMessageLevelIsNormalized(t *testing.T) {
	ctx := Add(context.Background(), New(" Alert Level! ", "Careful"))
	content := partial.NewID("content", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `{{ flash }}`,
	}))
	content.SetFunc(FuncMap())
	content.Use(Stage())

	out, err := content.Render(ctx)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := string(out); !strings.Contains(got, `data-flash-level="alert-level"`) {
		t.Fatalf("expected normalized level, got %s", got)
	}
}

func TestAddWithoutMessagesDoesNotCreateStore(t *testing.T) {
	ctx := Add(context.Background())
	if store := FromContext(ctx); store != nil {
		t.Fatalf("expected no store, got %#v", store)
	}
}

func TestMessagesSnapshotDoesNotMutateStore(t *testing.T) {
	store := NewStore(Success("Saved"))
	messages := store.Messages()
	messages[0].Text = "changed"

	if got := store.Messages()[0].Text; got != "Saved" {
		t.Fatalf("expected store snapshot isolation, got %q", got)
	}
}

func TestRendererDoesNotBleedConcurrentMessages(t *testing.T) {
	content := partial.NewID("content", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `{{ flash }}`,
	}))
	content.SetFunc(FuncMap())
	content.Use(Stage())

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			text := fmt.Sprintf("message-%02d", i)
			ctx := Add(context.Background(), Info(text))
			out, err := content.Render(ctx)
			if err != nil {
				errs <- err.Error()
				return
			}
			html := string(out)
			if !strings.Contains(html, text) {
				errs <- "missing " + text + " in " + html
				return
			}
			for j := 0; j < workers; j++ {
				other := fmt.Sprintf("message-%02d", j)
				if j != i && strings.Contains(html, other) {
					errs <- "unexpected " + other + " in " + html
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestStoreDrain(t *testing.T) {
	ctx := Add(context.Background(), Info("One"), Success("Two"))
	if !Has(ctx) {
		t.Fatal("expected messages")
	}
	drained := Drain(ctx)
	if len(drained) != 2 {
		t.Fatalf("expected 2 drained messages, got %d", len(drained))
	}
	if Has(ctx) {
		t.Fatal("expected store to be empty after drain")
	}
}

func TestStoreConcurrentAdd(t *testing.T) {
	store := NewStore()
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			store.Add(Info("loaded"))
		}()
	}
	wg.Wait()
	if got := len(store.Messages()); got != workers {
		t.Fatalf("expected %d messages, got %d", workers, got)
	}
}

func testFS(files map[string]string) fs.FS {
	out := make(fstest.MapFS, len(files))
	for name, body := range files {
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}
