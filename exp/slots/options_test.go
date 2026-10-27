package slots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/exp/target"
)

type comment struct {
	Text string
}

func pageFS() fstest.MapFS {
	return fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<body>{{ slot "cart" }}{{ content }}</body>`)},
		"content.gohtml": {Data: []byte(`<main>{{ slot "comments" }}</main>`)},
		"cart.gohtml":    {Data: []byte(`<span id="cart"{{ oobAttr }}>2</span>`)},
		"comments.gohtml": {
			Data: []byte("{{/*\n@dot github.com/donseba/go-partial/exp/slots.comment\n*/}}<ul id=\"comments\">{{ .Text }}</ul>"),
		},
	}
}

func newPage(dot any) *partial.Partial {
	content := partial.NewID("content", "content.gohtml")
	root := partial.NewID("root", "layout.gohtml").
		SetFileSystem(pageFS()).
		SetConnector(connector.NewHTMX(nil)).
		UseTemplateCache(true).
		Use(Stage(), target.Stage()).
		SetContent(content).
		SetTargetFallback(partial.TargetFallback{ID: "content"})

	Set(root, "cart", partial.NewID("cart", "cart.gohtml"), OOB())
	Set(root, "comments", partial.NewID("comments", "comments.gohtml").SetDot(dot), CheckContracts(), OmitOOB())

	return root
}

func targetRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", id)
	return req
}

func TestSlotsAreInheritedFromAncestors(t *testing.T) {
	out, err := partial.Render(context.Background(), newPage(comment{Text: "hi"}))
	if err != nil {
		t.Fatal(err)
	}

	want := `<body><span id="cart">2</span><main><ul id="comments">hi</ul></main></body>`
	if string(out) != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}
}

func TestNearerSlotReplacesAnAncestorsSlot(t *testing.T) {
	root := newPage(comment{Text: "root"})
	content := partial.NewID("content", "content.gohtml")
	root.SetContent(content)
	Set(content, "comments", partial.NewID("comments", "comments.gohtml").SetDot(comment{Text: "own"}))

	out, err := partial.Render(context.Background(), root)
	if err != nil || !strings.Contains(string(out), `<ul id="comments">own</ul>`) {
		t.Fatalf("Render() = %q, %v", out, err)
	}
}

func TestStandaloneSlotAnswersOnItsOwn(t *testing.T) {
	out, err := partial.RenderWithRequest(context.Background(), targetRequest("comments"), newPage(comment{Text: "hi"}))
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != `<ul id="comments">hi</ul>` {
		t.Fatalf("RenderWithRequest() = %q, want the slot alone", out)
	}
}

func TestStandaloneSlotThatCannotRenderFallsBack(t *testing.T) {
	out, err := partial.RenderWithRequest(context.Background(), targetRequest("comments"), newPage("not a comment"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(out), "<main>") {
		t.Fatalf("RenderWithRequest() = %q, want the content of the target fallback", out)
	}
}

func TestOOBSlotFollowsEveryResponse(t *testing.T) {
	out, err := partial.RenderWithRequest(context.Background(), targetRequest("content"), newPage(comment{Text: "hi"}))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `<span id="cart" hx-swap-oob="true">2</span>`) {
		t.Fatalf("RenderWithRequest() = %q, want the cart out of band", out)
	}
}

// TestOOBSlotGoesOnceWithItsOwner checks that a partial request for the
// partial an out-of-band slot is set on gets the slot where its template
// places it, and not again out of band.
func TestOOBSlotGoesOnceWithItsOwner(t *testing.T) {
	out, err := partial.RenderWithRequest(context.Background(), targetRequest("root"), newPage(comment{Text: "hi"}))
	if err != nil {
		t.Fatal(err)
	}

	want := `<body><span id="cart">2</span><main><ul id="comments">hi</ul></main></body>`
	if string(out) != want {
		t.Fatalf("RenderWithRequest() = %q, want %q", out, want)
	}
}

func TestSetUnderAParentLeavesTheParentAlone(t *testing.T) {
	shared := partial.NewID("shared").Use(Stage())
	Set(shared, "cart", partial.NewID("cart", "cart.gohtml"))

	page := partial.NewID("page", "layout.gohtml").SetParent(shared)
	Set(page, "comments", partial.NewID("comments", "comments.gohtml"))

	if !HasSlot("cart", &partial.RenderContext{Partial: page}) || !HasSlot("comments", &partial.RenderContext{Partial: page}) {
		t.Fatal("the page should have its own slot and its parent's")
	}

	if HasSlot("comments", &partial.RenderContext{Partial: shared}) {
		t.Fatal("Set on a partial changed the slots of its parent")
	}
}

func TestSetOnACloneLeavesTheOriginalAlone(t *testing.T) {
	original := partial.NewID("page")
	Set(original, "cart", partial.NewID("cart", "cart.gohtml"))

	clone := original.Clone()
	Set(clone, "comments", partial.NewID("comments", "comments.gohtml"))

	if !HasSlot("cart", &partial.RenderContext{Partial: clone}) || !HasSlot("comments", &partial.RenderContext{Partial: clone}) {
		t.Fatal("the clone should keep the original's slot and have its own")
	}

	if HasSlot("comments", &partial.RenderContext{Partial: original}) {
		t.Fatal("Set on a clone changed the slots of the original")
	}
}
