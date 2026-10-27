package partial

import (
	"html/template"
	"net/http"
	"slices"
	"strings"

	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/internal/templateutil"
)

// OOBElements is what the function of SetOOBElementsFunc chooses for a
// request: the elements of a region that go out of band, and the request
// headers the choice depends on.
type OOBElements struct {
	// IDs are the IDs of the elements that go out of band, as
	// SetOOBElements takes them; none sends the whole output.
	IDs []string
	// Vary names the request headers the choice depends on, such as the
	// header in which the client library sends the URL of the page it
	// shows (connector.CurrentURLHeader). Write adds them to the Vary
	// header of the response, so caches keep the responses apart; names
	// that are empty are left out.
	Vary []string
}

// SetOOBElements makes the partial, when it goes out of band with a
// partial response (see WithOOB and SetAlwaysSwapOOB), send only the
// elements of its output with these IDs, each swapped out of band on its
// own with the connector's attributes (see connector.OOBAttrs), rather
// than its whole output. It suits a region of which a request changes a
// few elements the application can name, such as the items of a menu that
// mark the page being shown:
//
//	menu.SetOOBElements("menu-about", "menu-contact")
//
// An element inside another one it sends goes along with that one, and
// an ID that is not the ID of an element in the output is left out. When
// none of them is, or when the end of an element cannot be told, such as
// an <li> without its </li>, the whole output goes, as it does without
// IDs: a region that does not give its elements those IDs keeps working.
// The region's own element, such as the one that carries {{oobAttr}}, is
// not sent unless its ID is one of them. Rendered on a full page, the
// partial is not affected. It replaces SetOOBElementsFunc, and
// SetOOBElementsFunc replaces it.
func (p *Partial) SetOOBElements(ids ...string) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.oobElements = slices.Clip(ids)
	p.oobElementsFunc = nil

	return p
}

// SetOOBElementsFunc is SetOOBElements with the IDs chosen per request by
// fn, such as the menu items whose state changes from the page the
// visitor comes from to the page requested. fn is called only when the
// partial goes out of band, once per response, and not for a full page or
// a response that leaves the partial out. The headers it names in Vary go
// to the Vary header of the response then, and only then:
//
//	menu.SetOOBElementsFunc(func(r *http.Request) partial.OOBElements {
//		return partial.OOBElements{
//			IDs:  changedItems(connector.CurrentURL(conn, r), r.URL),
//			Vary: []string{connector.CurrentURLHeader(conn)},
//		}
//	})
//
// A nil fn sends the whole output again. It replaces SetOOBElements, and
// SetOOBElements replaces it.
func (p *Partial) SetOOBElementsFunc(fn func(r *http.Request) OOBElements) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.oobElements = nil
	p.oobElementsFunc = fn

	return p
}

// oobOutput is the output of an out-of-band region as it goes with a
// partial response to r: its elements with the IDs of SetOOBElements or
// SetOOBElementsFunc, or the whole output. vary names the request headers
// the choice depends on.
func (p *Partial) oobOutput(r *http.Request, html template.HTML) (out template.HTML, vary []string) {
	p.mu.RLock()
	ids := p.oobElements
	fn := p.oobElementsFunc
	p.mu.RUnlock()

	if fn != nil {
		chosen := fn(r)
		ids = chosen.IDs
		vary = chosen.Vary
	}

	if len(ids) == 0 {
		return html, vary
	}

	attrs := templateutil.HTMLAttrs(connector.OOBAttrs(p.getConnectorOrDefault(), ""))

	return selectElements(html, ids, attrs), vary
}

// selectElements returns the elements of html with the given IDs, each
// with attrs added to its start tag; see SetOOBElements.
func selectElements(html template.HTML, ids []string, attrs template.HTMLAttr) template.HTML {
	source := string(html)

	var out strings.Builder

	found := false

	for i := 0; i < len(source); {
		tag, next, ok := scanTag(source, i)
		if !ok {
			i = next

			continue
		}

		if tag.id == "" || !slices.Contains(ids, tag.id) {
			i = next

			continue
		}

		end, ok := elementEnd(source, tag)
		if !ok {
			return html
		}

		found = true

		out.WriteString(source[tag.start:tag.nameEnd])
		out.WriteString(string(attrs))
		out.WriteString(source[tag.nameEnd:end])

		// What the element holds goes along with it.
		i = end
	}

	if !found {
		return html
	}

	return template.HTML(out.String())
}

// startTag is a start tag found by scanTag.
type startTag struct {
	// name is the element name in lower case.
	name string
	// start is where the tag begins, nameEnd where its name ends and end
	// just after its ">".
	start   int
	nameEnd int
	end     int
	// id is the value of its id attribute, as written.
	id string
	// selfClosing is set for a tag ending in "/>".
	selfClosing bool
}

// scanTag reads the markup at source[i:]: a start tag, with ok set, or
// text, a comment, an end tag or a declaration, which it skips. next is
// where the scan goes on. A start tag of an element whose content is text,
// such as <script>, is skipped with that content.
func scanTag(source string, i int) (tag startTag, next int, ok bool) {
	open := strings.IndexByte(source[i:], '<')
	if open < 0 {
		return startTag{}, len(source), false
	}

	i += open
	rest := source[i:]

	switch {
	case strings.HasPrefix(rest, "<!--"):
		return startTag{}, skipPast(source, i+4, "-->"), false
	case strings.HasPrefix(rest, "</"), strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "<?"):
		return startTag{}, skipPast(source, i+1, ">"), false
	case len(rest) < 2 || !isLetter(rest[1]):
		return startTag{}, i + 1, false
	}

	tag = startTag{start: i}

	j := i + 1
	for j < len(source) && isNameByte(source[j]) {
		j++
	}

	tag.nameEnd = j
	tag.name = strings.ToLower(source[i+1 : j])

	if !scanAttributes(source, j, &tag) {
		return startTag{}, len(source), false
	}

	if rawText[tag.name] && !tag.selfClosing {
		next, _ := skipRawText(source, tag.end, tag.name)

		return tag, next, true
	}

	return tag, tag.end, true
}

// scanAttributes reads the attributes of a start tag from source[i:] up
// to and with its ">", and sets the tag's end, just after it, its id and
// whether it closes itself. It reports false when the tag does not end.
func scanAttributes(source string, i int, tag *startTag) bool {
	for i < len(source) {
		switch c := source[i]; {
		case c == '>':
			tag.end = i + 1

			return true
		case c == '/' && i+1 < len(source) && source[i+1] == '>':
			tag.end = i + 2
			tag.selfClosing = true

			return true
		case isSpace(c) || c == '/':
			i++

			continue
		}

		attr, ok := scanAttribute(source, i)
		if !ok {
			return false
		}

		if attr.hasValue && strings.EqualFold(attr.name, "id") {
			tag.id = attr.value
		}

		i = attr.next
	}

	return false
}

// attribute is an attribute of a start tag, read by scanAttribute.
type attribute struct {
	name  string
	value string
	// hasValue is set when the attribute has a value, after an "=".
	hasValue bool
	// next is where the scan of the tag goes on.
	next int
}

// scanAttribute reads the attribute at source[i:]: its name and, after an
// "=", its value, quoted or not. It reports false when a quoted value does
// not end.
func scanAttribute(source string, i int) (attribute, bool) {
	start := i
	for i < len(source) && !isSpace(source[i]) && source[i] != '=' && source[i] != '>' && source[i] != '/' {
		i++
	}

	attr := attribute{name: source[start:i]}

	i = skipSpaces(source, i)
	if i >= len(source) || source[i] != '=' {
		attr.next = i

		return attr, true
	}

	// A source that ends after the "=" holds a tag that does not end.
	i = skipSpaces(source, i+1)
	if i >= len(source) {
		attr.next = i

		return attr, true
	}

	value, next, ok := scanValue(source, i)
	if !ok {
		return attribute{}, false
	}

	attr.value = value
	attr.hasValue = true
	attr.next = next

	return attr, true
}

// scanValue reads the attribute value at source[i:], quoted or not, and
// returns it with the index just after it. It reports false when a quoted
// value does not end.
func scanValue(source string, i int) (value string, next int, ok bool) {
	quote := source[i]
	if quote == '"' || quote == '\'' {
		closing := strings.IndexByte(source[i+1:], quote)
		if closing < 0 {
			return "", 0, false
		}

		return source[i+1 : i+1+closing], i + closing + 2, true
	}

	start := i
	for i < len(source) && !isSpace(source[i]) && source[i] != '>' {
		i++
	}

	return source[start:i], i, true
}

// skipSpaces is the index of the first byte of source from i that is not
// whitespace, or the end of source.
func skipSpaces(source string, i int) int {
	for i < len(source) && isSpace(source[i]) {
		i++
	}

	return i
}

// elementEnd is where the element of tag ends, just after its end tag;
// ok is false when it has none.
func elementEnd(source string, tag startTag) (int, bool) {
	if tag.selfClosing || voidElements[tag.name] {
		return tag.end, true
	}

	if rawText[tag.name] {
		return skipRawText(source, tag.end, tag.name)
	}

	depth := 1

	for i := tag.end; i < len(source); {
		open := strings.IndexByte(source[i:], '<')
		if open < 0 {
			return 0, false
		}

		i += open

		if closing, ok := endTagName(source, i); ok {
			i = skipPast(source, i+2, ">")

			if closing == tag.name {
				depth--

				if depth == 0 {
					return i, true
				}
			}

			continue
		}

		inner, next, ok := scanTag(source, i)
		if ok && inner.name == tag.name && !inner.selfClosing {
			depth++
		}

		i = next
	}

	return 0, false
}

// endTagName is the lower-case name of the end tag at source[i:], if it is
// one.
func endTagName(source string, i int) (string, bool) {
	if !strings.HasPrefix(source[i:], "</") {
		return "", false
	}

	j := i + 2
	for j < len(source) && isNameByte(source[j]) {
		j++
	}

	if j == i+2 {
		return "", false
	}

	return strings.ToLower(source[i+2 : j]), true
}

// skipRawText skips the content of an element whose content is text, such
// as <script>, from i to just after its end tag; without one, to the end
// of source, and found is false.
func skipRawText(source string, i int, name string) (end int, found bool) {
	for i < len(source) {
		open := strings.Index(source[i:], "</")
		if open < 0 {
			break
		}

		i += open

		if closing, ok := endTagName(source, i); ok && closing == name {
			return skipPast(source, i+2, ">"), true
		}

		i += 2
	}

	return len(source), false
}

// skipPast is just after the first marker in source from i, or the end.
func skipPast(source string, i int, marker string) int {
	if i > len(source) {
		return len(source)
	}

	at := strings.Index(source[i:], marker)
	if at < 0 {
		return len(source)
	}

	return i + at + len(marker)
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func isNameByte(c byte) bool {
	return isLetter(c) || '0' <= c && c <= '9' || c == '-' || c == ':' || c == '_'
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// voidElements have no end tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true,
}

// rawText are the elements whose content is text, not markup.
var rawText = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true,
}
