package errors

import (
	"crypto/sha256"
	"encoding/base64"
	stderrors "errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

var (
	styleElement   = regexp.MustCompile(`(?s)<style>(.*?)</style>`)
	styleAttribute = regexp.MustCompile(`(?i)\sstyle\s*=`)
	classAttribute = regexp.MustCompile(`class="([^"]+)"`)
)

// hashOf computes a hash source the way a browser does, apart from the
// package's own code, so a test catches a hash that drifts from the text.
func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))

	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// renderBuiltInMarkup renders the built-in page or fragment through the
// stage, in mode.
func renderBuiltInMarkup(t *testing.T, mode Mode, name string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	ctx := &partial.RenderContext{
		Context: req.Context(),
		Request: req,
		URL:     req.URL,
		Partial: partial.New("broken.gohtml").ID("broken"),
		Kind:    RenderKindError,
		Name:    name,
		Error:   stderrors.New("template: broken.gohtml:1: unexpected EOF"),
	}

	out, err := Stage(WithMode(mode)).Render(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("Render(%d, %q) error = %v", mode, name, err)
	}

	return string(out)
}

// TestStyleHashMatchesRenderedPage checks that StyleHash is the hash of
// the text of the page's one <style> element, as it is written, in both
// modes.
func TestStyleHashMatchesRenderedPage(t *testing.T) {
	for _, mode := range []Mode{ModeSafe, ModeDetailed} {
		body := renderBuiltInMarkup(t, mode, "page")

		matches := styleElement.FindAllStringSubmatch(body, -1)
		if len(matches) != 1 {
			t.Fatalf("mode %d: %d <style> elements in %q", mode, len(matches), body)
		}

		if got, want := StyleHash(), hashOf(matches[0][1]); got != want {
			t.Fatalf("mode %d: StyleHash() = %s, the rendered style hashes to %s", mode, got, want)
		}
	}
}

// TestStyleHashIsPublished pins the hash applications put in their
// policies: when the page's style changes, say so in RELEASE_NOTES, since
// their policies must change with it.
func TestStyleHashIsPublished(t *testing.T) {
	if got, want := StyleHash(), "'sha256-5WNh79OqlbaM4rZ/DlkOnop5aYED/JCWp9ChTZ5ZOSQ='"; got != want {
		t.Fatalf("StyleHash() = %s, want %s; update the README and RELEASE_NOTES", got, want)
	}
}

// TestBuiltInMarkupHasNoStyleAttributes checks that neither the page nor
// the fragment has a style attribute, which style-src-attr blocks, and
// that the fragment has no <style> element either.
func TestBuiltInMarkupHasNoStyleAttributes(t *testing.T) {
	for _, mode := range []Mode{ModeSafe, ModeDetailed} {
		for _, name := range []string{"page", "fragment"} {
			body := renderBuiltInMarkup(t, mode, name)

			if styleAttribute.MatchString(body) {
				t.Fatalf("mode %d, %s has a style attribute: %q", mode, name, body)
			}

			if name == "fragment" && strings.Contains(body, "<style") {
				t.Fatalf("mode %d, fragment has a <style> element: %q", mode, body)
			}
		}
	}
}

// TestRespondWritesNoStyleAttributes checks the safe answers of Respond,
// the page and the fragment, as a response.
func TestRespondWritesNoStyleAttributes(t *testing.T) {
	root := partial.NewID("page", "page.gohtml").SetFileSystem(themeFS()).SetConnector(connector.NewHTMX4(nil))

	page := httptest.NewRecorder()
	Respond(page, httptest.NewRequest(http.MethodGet, "/", nil), root, 0, stderrors.New("failed"))

	fragment := httptest.NewRecorder()
	Respond(fragment, partialRequest(), root, http.StatusTooManyRequests, nil)

	for name, body := range map[string]string{"page": page.Body.String(), "fragment": fragment.Body.String()} {
		if styleAttribute.MatchString(body) {
			t.Fatalf("%s has a style attribute: %q", name, body)
		}
	}

	if !strings.Contains(fragment.Body.String(), `class="go-partial-error__title"`) {
		t.Fatalf("fragment = %q", fragment.Body.String())
	}
}

// TestStylesheetStylesEveryFragmentClass checks that StylesheetFS holds a
// stylesheet with a rule for each class of the fragment.
func TestStylesheetStylesEveryFragmentClass(t *testing.T) {
	data, err := fs.ReadFile(StylesheetFS(), StylesheetName)
	if err != nil {
		t.Fatalf("StylesheetFS: %v", err)
	}

	css := string(data)

	for _, match := range classAttribute.FindAllStringSubmatch(fragmentTemplate, -1) {
		for _, class := range strings.Fields(match[1]) {
			if !strings.Contains(css, "."+class+" ") {
				t.Fatalf("the stylesheet has no rule for .%s", class)
			}
		}
	}
}
