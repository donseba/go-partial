package partial

import (
	"context"
	"html/template"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Tests
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
		// Test cases with accented characters
		{"élan vital", "Élan Vital"},
		{"über cool", "Über Cool"},
		{"façade", "Façade"},
		{"mañana", "Mañana"},
		{"crème brûlée", "Crème Brûlée"},
		// Test cases with non-Latin scripts
		{"россия", "Россия"},                 // Russian (Cyrillic script)
		{"中国", "中国"},                         // Chinese characters
		{"こんにちは 世界", "こんにちは 世界"},             // Japanese (Hiragana and Kanji)
		{"مرحبا بالعالم", "مرحبا بالعالم"},   // Arabic script
		{"γειά σου κόσμε", "Γειά Σου Κόσμε"}, // Greek script
		// Test cases with mixed scripts
		{"hello 世界", "Hello 世界"},
		{"こんにちは world", "こんにちは World"},
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
		// Test cases with accented characters
		{"élan vital", "Élan vital"},
		{"über cool", "Über cool"},
		{"façade", "Façade"},
		{"mañana", "Mañana"},
		{"crème brûlée", "Crème brûlée"},
		// Test cases with non-Latin scripts
		{"россия", "Россия"},                 // Russian (Cyrillic script)
		{"中国", "中国"},                         // Chinese characters
		{"こんにちは 世界", "こんにちは 世界"},             // Japanese (Hiragana and Kanji)
		{"مرحبا بالعالم", "مرحبا بالعالم"},   // Arabic script
		{"γειά σου κόσμε", "Γειά σου κόσμε"}, // Greek script
		// Test cases with mixed scripts
		{"hello 世界", "Hello 世界"},
		{"こんにちは world", "こんにちは world"},
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

	expected := map[string]any{
		"name":  "Ada",
		"count": 2,
	}
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
	cases := []struct {
		input    map[string]any
		expected []string
	}{
		{map[string]any{"a": 1, "b": 2}, []string{"a", "b"}},
		{map[string]any{}, []string{}},
	}
	for _, c := range cases {
		output := keys(c.input)
		if !equalStringSlices(output, c.expected) {
			t.Errorf("keys(%v) = %v; want %v", c.input, output, c.expected)
		}
	}
}

// Helper function to compare slices regardless of order
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]int)
	bMap := make(map[string]int)
	for _, v := range a {
		aMap[v]++
	}
	for _, v := range b {
		bMap[v]++
	}
	return reflect.DeepEqual(aMap, bMap)
}

func TestDebugRendersDefaultDebugBox(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("debug.gohtml", `{{ debug .Data }}`)

	p := NewID("debug", "debug.gohtml").SetFileSystem(fsys).SetData(map[string]any{
		"a": 1,
		"b": "test",
	})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-debug"`) {
		t.Fatalf("expected styled debug box, got %q", body)
	}
	if !strings.Contains(body, `&#34;a&#34;: 1`) || !strings.Contains(body, `&#34;b&#34;: &#34;test&#34;`) {
		t.Fatalf("expected debug output to contain data, got %q", body)
	}
}

func TestDebugUsesCustomDebugRenderer(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("debug.gohtml", `{{ debug .Data.Name }}`)

	p := NewID("debug", "debug.gohtml").
		SetFileSystem(fsys).
		SetData(map[string]any{"Name": "Ada"}).
		SetDebugRenderer(func(ctx context.Context, p *Partial, data *Data, value any) (template.HTML, error) {
			return template.HTML(`<aside class="custom-debug">` + value.(string) + `</aside>`), nil
		})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="custom-debug">Ada</aside>` {
		t.Fatalf("unexpected custom debug output: %q", out)
	}
}

func TestChildDebugRendererSurvivesClone(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("parent.gohtml", `{{ child "child" }}`)
	fsys.AddFile("child.gohtml", `{{ debug .Data.Name }}`)

	child := NewID("child", "child.gohtml").
		SetFileSystem(fsys).
		SetData(map[string]any{"Name": "Ada"}).
		SetDebugRenderer(func(ctx context.Context, p *Partial, data *Data, value any) (template.HTML, error) {
			return template.HTML(`<aside class="child-debug">` + value.(string) + `</aside>`), nil
		})

	parent := NewID("parent", "parent.gohtml").
		SetFileSystem(fsys).
		With(child)

	out, err := parent.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="child-debug">Ada</aside>` {
		t.Fatalf("expected child debug renderer to survive clone, got %q", out)
	}
}
