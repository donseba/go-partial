// Package head keeps the elements a page writes into the document's
// <head>, such as <link rel="canonical">, <meta name="description">,
// alternate and feed links and structured data, right when a partial
// response swaps only a part of the page.
//
// A region in <head> cannot go out of band as other regions do: <head>
// holds no element that could wrap it, and its elements change in number
// from page to page, such as the alternate links of the languages a page
// is in. Region makes a partial such a region: on a full page its output
// sits between two empty <template> markers, and out of band it replaces
// whatever is between them, as a group, so elements of the page left are
// removed rather than kept next to the new ones:
//
//	region := head.Region(partial.NewID("head", "regions/head.gohtml"))
//	slots.Set(root, "head", region, slots.OOB())
//
// With htmx (2 and 4) the group is replaced with two out-of-band swaps that
// need no extension: a delete of the elements between the markers and an
// insert before the end marker. A <title> in the region's output goes on
// its own, outside the markers, where htmx takes it for the document's
// title. Connectors whose out-of-band attribute is not hx-swap-oob, such as
// Unpoly's, get the region's output as it is.
//
// A back or forward navigation that htmx restores from its history replaces
// the body and the title only, so the head keeps the region's elements of
// the page left. With htmx 2, historyCacheSize 0 and refreshOnHistoryMiss
// true in its configuration make such a navigation load the whole page.
package head

import (
	"html/template"
	"reflect"
	"strings"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

const (
	// StartAttr marks the empty <template> before the region's elements,
	// with the region's ID as value.
	StartAttr = "data-partial-head"
	// EndAttr marks the empty <template> after them.
	EndAttr = "data-partial-head-end"
)

// packageMarker is a type of this package's own, for packagePath.
type packageMarker struct{}

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[packageMarker]().PkgPath()

// Region makes p, a region of a layout placed in <head>, replace the
// elements it wrote before when it goes out of band, rather than being
// swapped by ID; see the package documentation. p's own render stages get
// a stage that writes the markers and the swaps; partials p renders are not
// affected.
func Region(p *partial.Partial) *partial.Partial {
	if p == nil {
		return nil
	}

	id := p.PartialID()

	return p.Use(partial.RenderStageHooks{
		Package: packagePath,
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			out, err := next(ctx)
			if err != nil || ctx == nil || ctx.Partial == nil || ctx.Partial.PartialID() != id {
				return out, err
			}

			if ctx.Partial.IsOOB() {
				return oob(ctx.Partial.Connector(), id, out), nil
			}

			return page(id, out), nil
		},
	})
}

// page is the output of the head region id on a full page: html between
// the markers, after its <title>, if it has one.
func page(id string, html template.HTML) template.HTML {
	title, rest := cutTitle(string(html))
	token := markerValue(id)

	var b strings.Builder

	b.WriteString(title)
	b.WriteString(`<template ` + StartAttr + `="` + token + `"></template>`)
	b.WriteString(rest)
	b.WriteString(`<template ` + EndAttr + `="` + token + `"></template>`)

	//nolint:gosec // html is the region's own output; the markers are made of safe characters.
	return template.HTML(b.String())
}

// oob is the output of the head region id out of band with conn: its
// <title>, if it has one, and two out-of-band swaps that remove the
// elements between the markers and add the rest of html before the end
// marker. A connector whose out-of-band attribute is not hx-swap-oob gets
// html as it is.
func oob(conn connector.Connector, id string, html template.HTML) template.HTML {
	if _, ok := connector.OOBAttrs(conn, "")["hx-swap-oob"]; !ok {
		return html
	}

	title, rest := cutTitle(string(html))
	token := markerValue(id)
	start := `[` + StartAttr + `="` + token + `"]`
	end := `[` + EndAttr + `="` + token + `"]`

	// The elements after the start marker that are not the end marker or
	// after it. The selectors hold no whitespace and no comma, which htmx
	// would read as a swap modifier or as a list of selectors.
	between := start + `~:not(` + end + `):not(` + end + `~*)`

	var b strings.Builder

	b.WriteString(title)
	b.WriteString(`<div hx-swap-oob="` + template.HTMLEscapeString("delete:"+between) + `"></div>`)
	b.WriteString(`<div hx-swap-oob="` + template.HTMLEscapeString("beforebegin:"+end) + `">`)
	b.WriteString(rest)
	b.WriteString(`</div>`)

	//nolint:gosec // html is the region's own output; the attributes are escaped.
	return template.HTML(b.String())
}

// markerValue is id with every character other than an ASCII letter, a
// digit, "-" or "_" replaced by "_", so it can stand in a selector of an
// out-of-band swap.
func markerValue(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, id)
}

// cutTitle takes the first <title> element out of html, which is not in a
// comment, a <script> or a <style>.
func cutTitle(html string) (title, rest string) {
	lower := asciiLower(html)

	for i := 0; i < len(lower); {
		open := strings.IndexByte(lower[i:], '<')
		if open < 0 {
			break
		}

		i += open

		switch {
		case strings.HasPrefix(lower[i:], "<!--"):
			i = skipPast(lower, i+4, "-->")
		case isTag(lower, i, "script"):
			i = skipPast(lower, i+1, "</script")
		case isTag(lower, i, "style"):
			i = skipPast(lower, i+1, "</style")
		case isTag(lower, i, "title"):
			end := skipPast(lower, i+1, "</title")
			end = skipPast(lower, end, ">")

			return html[i:end], html[:i] + html[end:]
		default:
			i++
		}
	}

	return "", html
}

// asciiLower is s with the ASCII letters in lower case, byte for byte, so
// an index in it is one in s.
func asciiLower(s string) string {
	b := []byte(s)

	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}

	return string(b)
}

// isTag reports whether a start tag named name begins at lower[i:].
func isTag(lower string, i int, name string) bool {
	rest := lower[i+1:]
	if !strings.HasPrefix(rest, name) || len(rest) == len(name) {
		return false
	}

	switch rest[len(name)] {
	case '>', ' ', '\t', '\n', '\r', '\f', '/':
		return true
	default:
		return false
	}
}

// skipPast is just after the first marker in s from i, or the end of s.
func skipPast(s string, i int, marker string) int {
	if i >= len(s) {
		return len(s)
	}

	at := strings.Index(s[i:], marker)
	if at < 0 {
		return len(s)
	}

	return i + at + len(marker)
}
