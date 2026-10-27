package interactions

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func TestSubmitLiveAndClientAttrs(t *testing.T) {
	tests := []struct {
		name      string
		connector connector.Connector
		template  string
		want      string
	}{
		{
			name:      "submit targets the form",
			connector: connector.NewHTMX4(nil),
			template:  `<form {{ submitAttrs runtime "/comments" }}>`,
			want:      `<form  hx-post="/comments" hx-swap="innerHTML" hx-target="this">`,
		},
		{
			name:      "submit with a target, swap, method and select",
			connector: connector.NewHTMX(nil),
			template:  `<form {{ submitAttrs runtime "/poll" "target" "#poll" "swap" "outerHTML" "method" "put" "select" "#poll" }}>`,
			want:      `<form  hx-put="/poll" hx-select="#poll" hx-swap="outerHTML" hx-target="#poll">`,
		},
		{
			name:      "submit with unpoly",
			connector: connector.NewUnpoly(nil),
			template:  `<form {{ submitAttrs runtime "/comments" "target" "#comments" }}>`,
			want:      `<form  up-submit="" up-target="#comments">`,
		},
		{
			name:      "submit with turbo",
			connector: connector.NewTurbo(nil),
			template:  `<form {{ submitAttrs runtime "/comments" "target" "#comments" }}>`,
			want:      `<form  data-turbo-frame="comments">`,
		},
		{
			name:      "live search",
			connector: connector.NewHTMX4(nil),
			template:  `<input {{ liveAttrs runtime "/search" "target" "#content" "swap" "outerMorph" "include" "closest form" "pushURL" "true" }}>`,
			want:      `<input  hx-get="/search" hx-include="closest form" hx-push-url="true" hx-swap="outerMorph" hx-target="#content" hx-trigger="input changed delay:300ms, search">`,
		},
		{
			name:      "live search with unpoly",
			connector: connector.NewUnpoly(nil),
			template:  `<input {{ liveAttrs runtime "/search" "target" "#results" "delay" "0.5s" }}>`,
			want:      `<input  up-autosubmit="" up-target="#results" up-watch-delay="500">`,
		},
		{
			name:      "client attributes",
			connector: connector.NewHTMX4(nil),
			template:  `<a {{ clientAttrs runtime "confirm" "Log out?" "boost" "off" }}>`,
			want:      `<a  hx-boost="false" hx-confirm="Log out?">`,
		},
		{
			name:      "page settings with htmx 4",
			connector: connector.NewHTMX4(nil),
			template:  `<body{{ clientAttrs runtime "inherit" "true" "boost" "on" "target" "#content" "swap" "outerHTML" "disableWhileRequest" "find button" "keepOnServerError" true }}>`,
			want:      `<body hx-boost:inherited="target:#content swap:outerHTML" hx-disable:inherited="find button" hx-status:5xx:inherited="swap:none push:false">`,
		},
		{
			name:      "page settings with a swap modifier and reload on error",
			connector: connector.NewHTMX4(nil),
			template:  `<body{{ clientAttrs runtime "inherit" "true" "boost" "on" "target" "#content" "swap" "outerHTML showTarget:body" "reloadOnError" "true" }}>`,
			want:      `<body data-partial-reload-on-error="" hx-boost:inherited="target:#content swap:&#39;outerHTML showTarget:body&#39;">`,
		},
		{
			name:      "reload on error with turbo",
			connector: connector.NewTurbo(nil),
			template:  `<a{{ clientAttrs runtime "reloadOnError" true }}>`,
			want:      `<a data-partial-reload-on-error="">`,
		},
		{
			name:      "submit button confirm",
			connector: connector.NewHTMX4(nil),
			template:  `<button type="submit"{{ clientAttrs runtime "submitConfirm" "Delete for good?" }}>`,
			want:      `<button type="submit" data-partial-confirm="Delete for good?">`,
		},
		{
			name:      "no boost and confirm with turbo",
			connector: connector.NewTurbo(nil),
			template:  `<a{{ noBoost runtime }}{{ confirmAttr runtime "Sure?" }}>`,
			want:      `<a data-turbo="false" data-turbo-confirm="Sure?">`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"page.gohtml": &fstest.MapFile{Data: []byte(test.template)},
			}
			p := partial.NewID("page", "page.gohtml").
				SetConnector(test.connector).
				SetFileSystem(fsys).
				SetFunc(FuncMap())

			out, err := partial.Render(context.Background(), p)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if string(out) != test.want {
				t.Fatalf("Render() =\n%s\nwant\n%s", out, test.want)
			}
		})
	}
}

func TestClientAttrsReportsAnInvalidBoost(t *testing.T) {
	if out := ClientAttrs(nil, "boost", "maybe"); !strings.Contains(string(out), "data-partial-error") {
		t.Fatalf("ClientAttrs() = %q", out)
	}
}

func TestClientAttrsReportsAnInvalidFlag(t *testing.T) {
	if out := ClientAttrs(nil, "inherit", "maybe"); !strings.Contains(string(out), "data-partial-error") {
		t.Fatalf("ClientAttrs() = %q", out)
	}
}

func TestAttrsFuncMapHasOnlyTheAttributeHelpers(t *testing.T) {
	funcs := AttrsFuncMap()

	for _, generic := range []string{"async", "reveal", "poll", "on", "stream", "prefetch", "refresh"} {
		if _, ok := funcs[generic]; ok {
			t.Errorf("AttrsFuncMap() has %q", generic)
		}
	}

	all := FuncMap()
	for name := range funcs {
		if _, ok := all[name]; !ok {
			t.Errorf("AttrsFuncMap() has %q, which FuncMap() has not", name)
		}
	}

	for _, name := range []string{"revealAttrs", "submitAttrs", "clientAttrs", "noBoost", "confirmAttr"} {
		if _, ok := funcs[name]; !ok {
			t.Errorf("AttrsFuncMap() lacks %q", name)
		}
	}
}

func TestAttrsFuncMapLeavesPollToTheApplication(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ poll }}<a{{ noBoost runtime }}>`)},
	}

	p := partial.NewID("page", "page.gohtml").
		SetConnector(connector.NewHTMX4(nil)).
		SetFileSystem(fsys).
		SetFunc(AttrsFuncMap()).
		SetFunc(map[string]any{"poll": func() string { return "votes" }})

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `votes<a hx-boost="false">` {
		t.Fatalf("Render() = %q", out)
	}
}
