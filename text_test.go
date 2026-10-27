package partial

import (
	"context"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"
)

type textSite struct {
	Name string
}

func textFS() fstest.MapFS {
	return fstest.MapFS{
		"mail/welcome.gohtml": {Data: []byte("{{/*\n@model Site github.com/donseba/go-partial.textSite\n*/}}" +
			`{{ define "subject" }}Welcome to {{ Site.Name }}, {{ .Name }}{{ end }}` +
			`{{ define "text" }}Hi {{ .Name }} & co,{{ template "signature" . }} {{ shout "bye" }} {{ greet }}{{ end }}` +
			`{{ define "html" }}<p>Hi {{ .Name }} & co</p>{{ end }}`)},
		"mail/partials/signature.gohtml": {Data: []byte(`{{ define "signature" }} -- {{ Site.Name }}{{ end }}`)},
	}
}

func TestRenderTextSharesTheTreeOfAnHTMLRender(t *testing.T) {
	for _, cached := range []bool{false, true} {
		root := NewID("root").
			SetFileSystem(textFS()).
			UseTemplateCache(cached).
			SetModel(textSite{Name: "Acme"}).
			SetFunc(template.FuncMap{"shout": func(ctx context.Context, s string) string { return strings.ToUpper(s) }}).
			Use(RenderStageHooks{
				FuncNames: []string{"greet"},
				ResolveFunc: func(rc *RenderContext, name string) (any, bool) {
					return func() string { return "kind=" + string(rc.Kind) }, name == "greet"
				},
			})

		mail := NewID("mail", "mail/welcome.gohtml").
			SetParent(root).
			SetIncludes("mail/partials/*.gohtml").
			SetDot(map[string]string{"Name": "Ann <ann@example.com>"})

		for range 2 {
			subject, err := RenderText(context.Background(), mail.SetEntry("subject"))
			if err != nil || subject != "Welcome to Acme, Ann <ann@example.com>" {
				t.Fatalf("cache %v: subject = %q, %v", cached, subject, err)
			}

			text, err := RenderText(context.Background(), mail.SetEntry("text"))
			if err != nil || text != "Hi Ann <ann@example.com> & co, -- Acme BYE kind=text" {
				t.Fatalf("cache %v: text = %q, %v", cached, text, err)
			}

			html, err := RenderSelf(context.Background(), nil, mail.SetEntry("html"))
			if err != nil || string(html) != "<p>Hi Ann &lt;ann@example.com&gt; & co</p>" {
				t.Fatalf("cache %v: html = %q, %v", cached, html, err)
			}
		}
	}
}

func TestRenderTextReportsErrors(t *testing.T) {
	if _, err := RenderText(context.Background(), nil); err == nil {
		t.Fatal("RenderText(nil) should fail")
	}

	root := NewID("mail", "mail/welcome.gohtml").SetFileSystem(textFS()).SetEntry("subject")
	if _, err := RenderText(context.Background(), root); err == nil || !strings.Contains(err.Error(), "Site") {
		t.Fatalf("RenderText() error = %v, want the missing model", err)
	}
}
