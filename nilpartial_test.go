package partial

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donseba/go-partial/connector"
)

// TestNilPartialIsSafe checks that the methods of a nil partial do nothing:
// the setters return nil, and the getters their zero values.
func TestNilPartialIsSafe(t *testing.T) {
	var p *Partial

	setters := map[string]*Partial{
		"SetTemplates":       p.SetTemplates("page.gohtml"),
		"SetEntry":           p.SetEntry("body"),
		"SetParent":          p.SetParent(New("page.gohtml")),
		"Clone":              p.Clone(),
		"SetExtension":       p.SetExtension("key", "value"),
		"Use":                p.Use(RenderStageHooks{}),
		"SetBasePath":        p.SetBasePath("/app"),
		"SetDot":             p.SetDot("dot"),
		"ClearDot":           p.ClearDot(),
		"SetContract":        p.SetContract("model", "value"),
		"SetModel":           p.SetModel("value"),
		"SetResponseHeaders": p.SetResponseHeaders(map[string]string{"X-A": "b"}),
		"SetStatus":          p.SetStatus(http.StatusTeapot),
		"SetResponse":        p.SetResponse(connector.Response{}),
		"SetEvents":          p.SetEvents(nil),
		"SetConnector":       p.SetConnector(connector.NewHTMX(nil)),
		"SetAlwaysSwapOOB":   p.SetAlwaysSwapOOB(true),
		"SetFunc":            p.SetFunc(template.FuncMap{}),
		"SetFileSystem":      p.SetFileSystem(Sources{}),
		"UseTemplateCache":   p.UseTemplateCache(true),
		"With":               p.With(New("card.gohtml")),
		"SetContent":         p.SetContent(New("card.gohtml")),
		"WithOOB":            p.WithOOB(New("card.gohtml")),
		"Parent":             p.Parent(),
	}

	for name, got := range setters {
		if got != nil {
			t.Errorf("nil partial %s() = %p, want nil", name, got)
		}
	}

	if p.PartialID() != "" || p.GetBasePath() != "" || p.TemplatePaths() != nil || p.IsOOB() {
		t.Error("nil partial has an ID, base path, templates or OOB state")
	}

	if value, ok := p.Extension("key"); ok || value != nil {
		t.Errorf("nil partial Extension() = %v, %v", value, ok)
	}

	if p.getStatus() != 0 || p.getResponseHeaders() != nil || p.getConnectorResponseHeaders() != nil {
		t.Error("nil partial has a status or response headers")
	}

	if p.getConnector() != nil || p.getEvents() != nil || p.getFS() == nil {
		t.Error("nil partial getters are wrong")
	}

	if got := p.Connector().GetTargetHeader(); got != "X-Target" {
		t.Errorf("nil partial Connector() target header = %q, want the partial connector's", got)
	}

	if got := p.Response().Value(); got != (connector.Response{}) {
		t.Errorf("nil partial Response() = %+v", got)
	}

	if p.isPartialRequest(httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Error("nil partial answers a partial request")
	}

	p.emitWithContext(t.Context(), nil, Event{Kind: EventRenderStart})

	applyRenderResponseHeaders(nil, &RenderResponse{})
	applyRenderResponseHeaders(httptest.NewRecorder(), nil)
}

// TestNilChildrenAreIgnored checks that adding a nil child leaves the
// partial as it was.
func TestNilChildrenAreIgnored(t *testing.T) {
	p := NewID("page", "page.gohtml")

	if p.With(nil) != p || p.SetContent(nil) != p || p.WithOOB(nil) != p {
		t.Fatal("adding a nil child did not return the partial")
	}

	if len(p.children) != 0 || len(p.oobChildren) != 0 {
		t.Fatalf("nil children were added: %v %v", p.children, p.oobChildren)
	}

	if p.SetContract("") != p {
		t.Fatal("SetContract() without an annotation did not return the partial")
	}
}

// TestExtensionsOfManyKeysAndParents checks that a partial keeps several
// extensions, that a key set again is replaced, that a clone keeps them
// apart from the original, and that a child finds those of its parents.
func TestExtensionsOfManyKeysAndParents(t *testing.T) {
	type first struct{}

	type second struct{}

	type third struct{}

	parent := NewID("page", "page.gohtml").
		SetExtension(first{}, "one").
		SetExtension(second{}, "two").
		SetExtension(first{}, "one again")

	if value, _ := parent.Extension(first{}); value != "one again" {
		t.Fatalf("first = %v, want the value set last", value)
	}

	if value, _ := parent.Extension(second{}); value != "two" {
		t.Fatalf("second = %v", value)
	}

	clone := parent.Clone()
	clone.SetExtension(second{}, "changed")

	if value, _ := parent.Extension(second{}); value != "two" {
		t.Fatalf("the clone changed the original: second = %v", value)
	}

	child := NewID("card", "card.gohtml")
	parent.With(child)

	if value, ok := child.Extension(second{}); !ok || value != "two" {
		t.Fatalf("child Extension(second) = %v, %v, want the parent's", value, ok)
	}

	if value, ok := child.Extension(third{}); ok || value != nil {
		t.Fatalf("child Extension(third) = %v, %v, want none", value, ok)
	}
}

func TestWithTemplateInfersTheChildID(t *testing.T) {
	tests := map[string]string{
		"templates/sidebar.gohtml": "sidebar",
		`templates\menu.gohtml`:    "menu",
		"templates/README":         "README",
		"/":                        "",
		"":                         "",
	}

	for path, want := range tests {
		if got := inferTemplateID(path); got != want {
			t.Errorf("inferTemplateID(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestNavigationHelpers checks the URL helpers templates get, for a render
// of a request and one without.
func TestNavigationHelpers(t *testing.T) {
	src := `{{ urlIs "items/" }} {{ urlStarts "/it" }} {{ urlContains "tem" }} ` +
		`{{ urlPath "/shop" "items" "4" }} {{ joinPath "a" "b" }} {{ oob }}`

	p := NewID("page", "page.gohtml").SetFileSystem(Sources{"page.gohtml": src})

	req := httptest.NewRequest(http.MethodGet, "/items", nil)

	out, err := RenderWithRequest(t.Context(), req, p)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if want := "true true true /shop/items/4 a/b false"; string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	out, err = Render(t.Context(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if want := "false false false /shop/items/4 a/b false"; string(out) != want {
		t.Fatalf("output without a request = %q, want %q", out, want)
	}
}
