package templateutil

import (
	"html/template"
	"maps"
	"slices"
	"strings"
	"testing"
)

func upper(s string) string {
	return strings.ToUpper(s)
}

func lower(s string) string {
	return strings.ToLower(s)
}

func TestNamesListsEveryFunction(t *testing.T) {
	names := Names(template.FuncMap{"upper": upper, "lower": lower})

	want := map[string]struct{}{"upper": {}, "lower": {}}
	if !maps.Equal(names, want) {
		t.Fatalf("Names() = %v, want %v", names, want)
	}

	if got := Names(nil); len(got) != 0 {
		t.Fatalf("Names(nil) = %v, want empty", got)
	}
}

// TestMergeFuncMapsLetsLaterMapsWin checks that merging keeps every name,
// that a later map replaces a function of an earlier one, and that the
// inputs are left as they were.
func TestMergeFuncMapsLetsLaterMapsWin(t *testing.T) {
	first := template.FuncMap{"case": upper, "upper": upper}
	second := template.FuncMap{"case": lower, "lower": lower}

	merged := MergeFuncMaps(first, nil, second)

	if got := slices.Sorted(maps.Keys(merged)); !slices.Equal(got, []string{"case", "lower", "upper"}) {
		t.Fatalf("merged names = %v", got)
	}

	fn, ok := merged["case"].(func(string) string)
	if !ok {
		t.Fatalf("merged case is %T", merged["case"])
	}

	if got := fn("MiXeD"); got != "mixed" {
		t.Fatalf("merged case(MiXeD) = %q, want the later map's lower", got)
	}

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("inputs changed: %v %v", first, second)
	}

	if got := MergeFuncMaps(); len(got) != 0 {
		t.Fatalf("MergeFuncMaps() = %v, want empty", got)
	}
}

func TestFunctionNameSignatures(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "func map",
			got:  FunctionNameSignature(template.FuncMap{"upper": upper, "lower": lower}),
			want: "lower;upper;",
		},
		{
			name: "empty func map",
			got:  FunctionNameSignature(nil),
			want: "",
		},
		{
			name: "set",
			got:  FunctionNameSignatureFromSet(map[string]struct{}{"b": {}, "a": {}}),
			want: "a;b;",
		},
		{
			name: "names sorted without repeats or empty names",
			got:  FunctionNameSignatureFromNames([]string{"c", "", "a", "c", "b"}),
			want: "a;b;c;",
		},
		{
			name: "no names",
			got:  FunctionNameSignatureFromNames(nil),
			want: "",
		},
		{
			name: "merged signatures",
			got:  MergeFunctionSignatures("b;a;", "", "c;a;"),
			want: "a;b;c;",
		},
		{
			name: "merged nothing",
			got:  MergeFunctionSignatures("", ";"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("signature = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestFunctionNameSignatureFromNamesKeepsTheInput checks that the names given
// are not sorted or compacted in place.
func TestFunctionNameSignatureFromNamesKeepsTheInput(t *testing.T) {
	names := []string{"b", "a", "b"}

	if got := FunctionNameSignatureFromNames(names); got != "a;b;" {
		t.Fatalf("signature = %q", got)
	}

	if !slices.Equal(names, []string{"b", "a", "b"}) {
		t.Fatalf("names changed to %v", names)
	}
}

func TestHTMLAttrsSortsAndEscapes(t *testing.T) {
	got := HTMLAttrs(map[string]string{
		"id":         "row-1",
		"data-title": `"Fish" & <Chips>`,
		"class":      "card",
	})

	want := ` class="card" data-title="&#34;Fish&#34; &amp; &lt;Chips&gt;" id="row-1"`
	if string(got) != want {
		t.Fatalf("HTMLAttrs() = %s, want %s", got, want)
	}

	if got := HTMLAttrs(nil); got != "" {
		t.Fatalf("HTMLAttrs(nil) = %q, want empty", got)
	}
}

func TestWriteAttrAppends(t *testing.T) {
	var b strings.Builder

	b.WriteString("<div")
	WriteAttr(&b, "title", "a<b")
	WriteAttr(&b, "hidden", "")

	if got, want := b.String(), `<div title="a&lt;b" hidden=""`; got != want {
		t.Fatalf("WriteAttr wrote %s, want %s", got, want)
	}
}
