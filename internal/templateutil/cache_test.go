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
	cached := NewCachedTemplate(base, map[string]struct{}{"value": {}})

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := strconv.Itoa(i)
			tmpl, release, err := cached.Template(template.FuncMap{
				"value": func() string { return want },
			})
			if err != nil {
				errs <- err.Error()
				return
			}
			defer release()

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
