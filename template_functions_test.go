package partial

import (
	"fmt"
	"html/template"
	"reflect"
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

func TestUcFirst(t *testing.T) {
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
		output := ucfirst(c.input)
		if output != c.expected {
			t.Errorf("ucfirst(%q) = %q; want %q", c.input, output, c.expected)
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
		output := formatDate(c.input, c.layout)
		if output != c.expected {
			t.Errorf("formatDate(%v, %q) = %q; want %q", c.input, c.layout, output, c.expected)
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

func TestDebug(t *testing.T) {
	input := map[string]any{"a": 1, "b": "test"}
	expected := fmt.Sprintf("%+v", input)
	output := debug(input)
	if output != expected {
		t.Errorf("debug(%v) = %q; want %q", input, output, expected)
	}
}
