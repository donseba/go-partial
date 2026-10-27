package debug

import (
	"crypto/sha256"
	"encoding/base64"
	"html/template"
	"regexp"
	"strings"
	"testing"

	partial "github.com/donseba/go-partial"
)

// TestBoxStylesWithHashedStyleElement checks that the debug box has no
// style attributes, which style-src-attr blocks, and that StyleHash is the
// hash of the text of its one <style> element, as it is written.
func TestBoxStylesWithHashedStyleElement(t *testing.T) {
	ctx := &partial.RenderContext{
		Kind:    RenderKindDebug,
		Data:    map[string]any{"name": "Ada", "quote": `</style><b>`},
		Partial: partial.New("page.gohtml").ID("page"),
	}

	out, err := Stage().Render(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if regexp.MustCompile(`(?i)\sstyle\s*=`).MatchString(body) {
		t.Fatalf("debug box has a style attribute: %q", body)
	}

	matches := regexp.MustCompile(`(?s)<style>(.*?)</style>`).FindAllStringSubmatch(body, -1)
	if len(matches) != 1 {
		t.Fatalf("%d <style> elements in %q", len(matches), body)
	}

	sum := sha256.Sum256([]byte(matches[0][1]))
	if want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"; StyleHash() != want {
		t.Fatalf("StyleHash() = %s, the rendered style hashes to %s", StyleHash(), want)
	}

	for _, class := range []string{"go-partial-debug__header", "go-partial-debug__label", "go-partial-debug__partial", "go-partial-debug__value"} {
		if !strings.Contains(body, `class="`+class+`"`) || !strings.Contains(matches[0][1], "."+class+"{") {
			t.Fatalf("class %s is not used and styled: %q", class, body)
		}
	}
}
