package templatehelpers

import (
	"html/template"
	"reflect"
	"testing"
	"time"
)

func TestFuncMapReturnsCopy(t *testing.T) {
	funcs := FuncMap()
	funcs["dict"] = func() string { return "changed" }

	next := FuncMap()
	if _, ok := next["dict"].(func(...any) (map[string]any, error)); !ok {
		t.Fatalf("FuncMap() should return a copy, got %#v", next["dict"])
	}
}

func TestSafeHTML(t *testing.T) {
	input := "<p>Hello, World!</p>"
	expected := template.HTML("<p>Hello, World!</p>")
	output := safeHTML(input)
	if output != expected {
		t.Errorf("safeHTML(%q) = %q; want %q", input, output, expected)
	}
}

func TestTitle(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"hello world", "Hello World"},
		{"HELLO WORLD", "Hello World"},
		{"go is awesome", "Go Is Awesome"},
		{"", ""},
		{"hello wereld", "Hello Wereld"},
	}
	for _, c := range cases {
		output := title(c.input)
		if output != c.expected {
			t.Errorf("title(%q) = %q; want %q", c.input, output, c.expected)
		}
	}
}

func TestSubstr(t *testing.T) {
	cases := []struct {
		input    string
		start    int
		length   int
		expected string
	}{
		{"Hello, World!", 7, 5, "World"},
		{"Hello, World!", 0, 5, "Hello"},
		{"Hello, World!", 7, 20, "World!"},
		{"Hello, World!", 20, 5, ""},
		{"Hello, World!", 0, 0, ""},
	}
	for _, c := range cases {
		output := substr(c.input, c.start, c.length)
		if output != c.expected {
			t.Errorf("substr(%q, %d, %d) = %q; want %q", c.input, c.start, c.length, output, c.expected)
		}
	}
}

func TestUpperFirst(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"hello world", "Hello world"},
		{"Hello world", "Hello world"},
		{"h", "H"},
		{"", ""},
	}
	for _, c := range cases {
		output := upperFirst(c.input)
		if output != c.expected {
			t.Errorf("upperFirst(%q) = %q; want %q", c.input, output, c.expected)
		}
	}
}

func TestFormatDate(t *testing.T) {
	t1 := time.Date(2021, time.December, 31, 23, 59, 59, 0, time.UTC)
	cases := []struct {
		input    time.Time
		layout   string
		expected string
	}{
		{t1, "2006-01-02", "2021-12-31"},
		{t1, "Jan 2, 2006", "Dec 31, 2021"},
		{t1, time.RFC3339, "2021-12-31T23:59:59Z"},
	}
	for _, c := range cases {
		output := formatDate(c.layout, c.input)
		if output != c.expected {
			t.Errorf("formatDate(%q, %v) = %q; want %q", c.layout, c.input, output, c.expected)
		}
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		layout   string
		value    string
		expected time.Time
		wantErr  bool
	}{
		{"2006-01-02", "2021-12-31", time.Date(2021, time.December, 31, 0, 0, 0, 0, time.UTC), false},
		{"Jan 2, 2006", "Dec 31, 2021", time.Date(2021, time.December, 31, 0, 0, 0, 0, time.UTC), false},
		{"2006-01-02", "invalid date", time.Time{}, true},
	}
	for _, c := range cases {
		output, err := parseDate(c.layout, c.value)
		if (err != nil) != c.wantErr {
			t.Errorf("parseDate(%q, %q) error = %v; wantErr %v", c.layout, c.value, err, c.wantErr)
			continue
		}
		if !c.wantErr && !output.Equal(c.expected) {
			t.Errorf("parseDate(%q, %q) = %v; want %v", c.layout, c.value, output, c.expected)
		}
	}
}

func TestFirst(t *testing.T) {
	cases := []struct {
		input    []any
		expected any
	}{
		{[]any{1, 2, 3}, 1},
		{[]any{"a", "b", "c"}, "a"},
		{[]any{}, nil},
	}
	for _, c := range cases {
		output := first(c.input)
		if !reflect.DeepEqual(output, c.expected) {
			t.Errorf("first(%v) = %v; want %v", c.input, output, c.expected)
		}
	}
}

func TestLast(t *testing.T) {
	cases := []struct {
		input    []any
		expected any
	}{
		{[]any{1, 2, 3}, 3},
		{[]any{"a", "b", "c"}, "c"},
		{[]any{}, nil},
	}
	for _, c := range cases {
		output := last(c.input)
		if !reflect.DeepEqual(output, c.expected) {
			t.Errorf("last(%v) = %v; want %v", c.input, output, c.expected)
		}
	}
}

func TestHasKey(t *testing.T) {
	cases := []struct {
		input    map[string]any
		key      string
		expected bool
	}{
		{map[string]any{"a": 1, "b": 2}, "a", true},
		{map[string]any{"a": 1, "b": 2}, "c", false},
		{map[string]any{}, "a", false},
	}
	for _, c := range cases {
		output := hasKey(c.input, c.key)
		if output != c.expected {
			t.Errorf("hasKey(%v, %q) = %v; want %v", c.input, c.key, output, c.expected)
		}
	}
}

func TestDict(t *testing.T) {
	out, err := dict("name", "Ada", "count", 2)
	if err != nil {
		t.Fatalf("dict() error = %v", err)
	}

	expected := map[string]any{"name": "Ada", "count": 2}
	if !reflect.DeepEqual(out, expected) {
		t.Fatalf("dict() = %#v; want %#v", out, expected)
	}
}

func TestDictRejectsOddArgumentCount(t *testing.T) {
	_, err := dict("name")
	if err == nil {
		t.Fatal("expected odd argument count error")
	}
}

func TestDictRejectsNonStringKeys(t *testing.T) {
	_, err := dict(1, "Ada")
	if err == nil {
		t.Fatal("expected non-string key error")
	}
}

func TestKeys(t *testing.T) {
	out := keys(map[string]any{"a": 1, "b": 2})
	if !sameStrings(out, []string{"a", "b"}) {
		t.Fatalf("keys() = %v; want [a b]", out)
	}
}

func TestIncDec(t *testing.T) {
	if got := inc(10); got != 11 {
		t.Fatalf("inc(10) = %v; want 11", got)
	}
	if got := inc(10, 5); got != 15 {
		t.Fatalf("inc(10, 5) = %v; want 15", got)
	}
	if got := dec(10); got != 9 {
		t.Fatalf("dec(10) = %v; want 9", got)
	}
	if got := dec(10, 5); got != 5 {
		t.Fatalf("dec(10, 5) = %v; want 5", got)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}
