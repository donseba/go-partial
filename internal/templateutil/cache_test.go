package templateutil

import (
	"bytes"
	"html/template"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	texttemplate "text/template"
)

func TestCachedTemplateCanExecuteConcurrently(t *testing.T) {
	base := template.Must(template.New("page").Funcs(template.FuncMap{
		"value": func() string { return "" },
	}).Parse(`{{ value }}`))
	cached := NewCachedTemplate(base, map[string]struct{}{"value": {}}, nil)

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)

			tmpl, err := cached.Template(template.FuncMap{
				"value": func() string { return want },
			})
			if err != nil {
				errs <- err.Error()
				return
			}
			defer cached.Release(tmpl)

			var out bytes.Buffer
			if err := tmpl.Execute(&out, nil); err != nil {
				errs <- err.Error()
				return
			}
			if got := out.String(); got != want {
				errs <- "got " + got + " want " + want
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// TestCachedTemplateSizeHint checks that the size hint follows the last
// output of each entry, with room to grow, within maxSizeHint.
func TestCachedTemplateSizeHint(t *testing.T) {
	var none *CachedTemplate
	none.NoteSize("", 100)

	if got := none.SizeHint(""); got != 0 {
		t.Fatalf("nil entry hint %d", got)
	}

	cached := NewCachedTemplate(template.Must(template.New("page").Parse(`x`)), nil, nil)
	if got := cached.SizeHint(""); got != 0 {
		t.Fatalf("hint before a render %d", got)
	}

	cached.NoteSize("", 8000)
	cached.NoteSize("row", 80)

	if got := cached.SizeHint(""); got != 9000 {
		t.Fatalf("hint %d, want 9000", got)
	}

	if got := cached.SizeHint("row"); got != 90 {
		t.Fatalf("row hint %d, want 90", got)
	}

	cached.NoteSize("", 400)

	if got := cached.SizeHint(""); got != 450 {
		t.Fatalf("hint after a smaller output %d, want 450", got)
	}

	cached.NoteSize("", 64<<20)

	if got := cached.SizeHint(""); got != maxSizeHint {
		t.Fatalf("hint %d, want at most %d", got, maxSizeHint)
	}
}

// TestStoreKeepsTemplatesTextsAndMutexes checks what a store gives back for
// a key, and that a nil store and nil entries are safe.
func TestStoreKeepsTemplatesTextsAndMutexes(t *testing.T) {
	store := NewStore()

	if _, ok := store.Load("page"); ok {
		t.Fatal("Load() found an entry in an empty store")
	}

	cached := NewCachedTemplate(template.Must(template.New("page").Parse(`x`)), nil, nil)
	store.Store("page", cached)
	store.Store("none", nil)

	if got, ok := store.Load("page"); !ok || got != cached {
		t.Fatalf("Load() = %p, %v, want the stored entry", got, ok)
	}

	if _, ok := store.Load("none"); ok {
		t.Fatal("Load() found a nil entry")
	}

	if _, ok := store.Text("text"); ok {
		t.Fatal("Text() found a template in an empty store")
	}

	text := texttemplate.Must(texttemplate.New("text").Parse(`plain`))
	store.StoreText("text", text)
	store.StoreText("none", nil)

	if got, ok := store.Text("text"); !ok || got != text {
		t.Fatalf("Text() = %p, %v, want the stored template", got, ok)
	}

	if _, ok := store.Text("none"); ok {
		t.Fatal("Text() found a nil template")
	}

	first, again := store.Mutex("page"), store.Mutex("page")
	if first != again {
		t.Fatal("Mutex() gave two mutexes for one key")
	}

	if store.Mutex("page") == store.Mutex("other") {
		t.Fatal("Mutex() gave one mutex for two keys")
	}

	var none *Store
	none.Store("page", cached)
	none.StoreText("text", text)

	if _, ok := none.Load("page"); ok {
		t.Fatal("nil store Load() found an entry")
	}

	if _, ok := none.Text("text"); ok {
		t.Fatal("nil store Text() found a template")
	}

	if none.Mutex("page") == nil {
		t.Fatal("nil store Mutex() = nil")
	}
}

func TestCachedTemplateKeepsItsFunctionsAndContracts(t *testing.T) {
	required := map[string]struct{}{"money": {}}
	contracts := map[string]RootContract{"Site": {Annotation: "model", Type: "site.Site"}}
	cached := NewCachedTemplate(template.Must(template.New("page").Parse(`x`)), required, contracts)

	if got := cached.RequiredFuncs(); len(got) != 1 {
		t.Fatalf("RequiredFuncs() = %v", got)
	}

	if got := cached.RootContracts()["Site"]; got.Type != "site.Site" || got.Annotation != "model" {
		t.Fatalf("RootContracts()[Site] = %+v", got)
	}

	var none *CachedTemplate
	if none.RequiredFuncs() != nil || none.RootContracts() != nil {
		t.Fatal("nil entry has functions or contracts")
	}

	if _, err := none.Template(nil); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("nil entry Template() error = %v", err)
	}

	if _, err := none.TemplateResolving(nil); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("nil entry TemplateResolving() error = %v", err)
	}

	none.Release(nil)
	cached.Release(nil)
}

// TestCachedTemplateResolvesItsFunctions checks that TemplateResolving asks
// only for the functions the template calls, and renders with them.
func TestCachedTemplateResolvesItsFunctions(t *testing.T) {
	base := template.Must(template.New("page").Funcs(template.FuncMap{
		"greet": func() string {
			return ""
		},
	}).Parse(`{{ greet }}`))
	cached := NewCachedTemplate(base, map[string]struct{}{"greet": {}, "unknown": {}}, nil)

	var asked []string

	tmpl, err := cached.TemplateResolving(func(name string) (any, bool) {
		asked = append(asked, name)
		if name != "greet" {
			return nil, false
		}

		return func() string {
			return "hello"
		}, true
	})
	if err != nil {
		t.Fatal(err)
	}

	slices.Sort(asked)

	if !slices.Equal(asked, []string{"greet", "unknown"}) {
		t.Fatalf("resolve asked for %v", asked)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "hello" {
		t.Fatalf("rendered %q, want hello", got)
	}
}

// TestCachedTemplateReusesReleasedCopies checks that a released copy is
// handed out again, with the new functions, and that the base template is
// not executed.
func TestCachedTemplateReusesReleasedCopies(t *testing.T) {
	base := template.Must(template.New("page").Funcs(template.FuncMap{
		"value": func() string {
			return ""
		},
	}).Parse(`{{ value }}`))
	cached := NewCachedTemplate(base, nil, nil)

	render := func(want string) {
		t.Helper()

		tmpl, err := cached.Template(template.FuncMap{
			"value": func() string {
				return want
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		if err := tmpl.Execute(&out, nil); err != nil {
			t.Fatal(err)
		}

		if got := out.String(); got != want {
			t.Fatalf("rendered %q, want %q", got, want)
		}

		cached.Release(tmpl)
	}

	render("first")
	render("second")

	// The base can still be cloned: no copy given out was the base itself.
	if _, err := base.Clone(); err != nil {
		t.Fatalf("base was executed: %v", err)
	}
}

func TestCachedTemplateReportsCloneErrors(t *testing.T) {
	base := template.Must(template.New("page").Parse(`x`))

	var out bytes.Buffer
	if err := base.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}

	cached := NewCachedTemplate(base, nil, nil)
	if _, err := cached.Template(nil); err == nil || !strings.Contains(err.Error(), "error cloning cached template") {
		t.Fatalf("Template() of an executed base error = %v", err)
	}
}

func TestFilterFuncMapKeepsTheRequiredFunctions(t *testing.T) {
	funcs := template.FuncMap{
		"a": strings.ToUpper,
		"b": strings.ToLower,
	}

	if got := FilterFuncMap(funcs, nil); len(got) != 2 {
		t.Fatalf("FilterFuncMap(nil) = %v, want every function", got)
	}

	got := FilterFuncMap(funcs, map[string]struct{}{"a": {}, "missing": {}})
	if len(got) != 1 || got["a"] == nil {
		t.Fatalf("FilterFuncMap() = %v, want only a", got)
	}
}
