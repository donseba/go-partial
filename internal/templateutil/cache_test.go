package templateutil

import (
	"bytes"
	"html/template"
	"strconv"
	"sync"
	"testing"
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
