package templateutil

import (
	"html/template"
	"slices"
	"strings"
)

// HTMLAttrs writes attrs as HTML attributes, sorted by name, each after a
// space, with their values escaped.
func HTMLAttrs(attrs map[string]string) template.HTMLAttr {
	// The names are collected in a slice of their exact size, as renders
	// write the attributes of every out-of-band region this way.
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}

	slices.Sort(names)

	var b strings.Builder

	for _, name := range names {
		WriteAttr(&b, name, attrs[name])
	}

	return template.HTMLAttr(b.String())
}

// WriteAttr writes the attribute name="value" to b, after a space, with
// value escaped.
func WriteAttr(b *strings.Builder, name, value string) {
	b.WriteByte(' ')
	b.WriteString(name)
	b.WriteString(`="`)
	b.WriteString(template.HTMLEscapeString(value))
	b.WriteByte('"')
}
