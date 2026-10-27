package connector

import (
	"net/http"
	"reflect"
	"testing"
)

func TestAttrsForPageSettings(t *testing.T) {
	body := Attr{
		Inherit:             true,
		Boost:               BoostOn,
		Target:              "#content",
		Swap:                "outerHTML",
		DisableWhileRequest: "find button[type='submit']",
		KeepOnServerError:   true,
	}

	tests := []struct {
		name      string
		connector Connector
		attr      Attr
		want      map[string]string
	}{
		{"htmx 4 body", NewHTMX4(nil), body, map[string]string{
			"hx-boost:inherited":      "target:#content swap:outerHTML",
			"hx-disable:inherited":    "find button[type='submit']",
			"hx-status:5xx:inherited": "swap:none push:false",
		}},
		{"htmx 4 inherited target alone", NewHTMX4(nil), Attr{Inherit: true, Target: "#main"}, map[string]string{
			"hx-target:inherited": "#main",
		}},
		{
			"htmx 4 not inherited",
			NewHTMX4(nil),
			Attr{Boost: BoostOn, Target: "#content", KeepOnServerError: true},
			map[string]string{
				"hx-boost": "true", "hx-target": "#content", "hx-status:5xx": "swap:none push:false",
			},
		},
		{"htmx 2 body", NewHTMX(nil), body, map[string]string{
			"hx-boost": "true", "hx-target": "#content", "hx-swap": "outerHTML", "hx-disabled-elt": "find button[type='submit']",
		}},
		{"unpoly body", NewUnpoly(nil), body, map[string]string{
			"up-follow": "", "up-submit": "", "up-target": "#content", "up-disable": "find button[type='submit']",
		}},
		{"turbo submit confirm", NewTurbo(nil), Attr{SubmitConfirm: "Sure?"}, map[string]string{"data-turbo-confirm": "Sure?"}},
		{"htmx submit confirm", NewHTMX4(nil), Attr{SubmitConfirm: "Sure?"}, map[string]string{"data-partial-confirm": "Sure?"}},
		{
			"data attributes",
			NewPartial(nil),
			Attr{DisableWhileRequest: "button", KeepOnServerError: true, SubmitConfirm: "Sure?"},
			map[string]string{
				"data-disable": "button", "data-keep-on-server-error": "true", "data-partial-confirm": "Sure?",
			},
		},
	}

	for _, test := range tests {
		if got := Attrs(test.connector, test.attr); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: Attrs() = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestMergeAttrsReplacesTheConnectorsAttributes(t *testing.T) {
	existing := map[string]string{"hx-confirm": "Delete?", "hx-post": "/delete", "class": "form"}

	merged := MergeAttrs(NewHTMX4(nil), existing, Attr{Confirm: "Reset the votes?"})
	if !reflect.DeepEqual(merged, map[string]string{"hx-confirm": "Reset the votes?", "hx-post": "/delete", "class": "form"}) {
		t.Fatalf("MergeAttrs() = %v", merged)
	}

	if existing["hx-confirm"] != "Delete?" {
		t.Fatal("MergeAttrs changed existing")
	}

	removed := MergeAttrs(NewHTMX4(nil), existing, Attr{}, AttrConfirm)
	if !reflect.DeepEqual(removed, map[string]string{"hx-post": "/delete", "class": "form"}) {
		t.Fatalf("MergeAttrs(remove) = %v", removed)
	}

	inherited := MergeAttrs(NewHTMX4(nil), map[string]string{"hx-target:inherited": "#old"}, Attr{Target: "#new"})
	if !reflect.DeepEqual(inherited, map[string]string{"hx-target": "#new"}) {
		t.Fatalf("MergeAttrs(inherited) = %v", inherited)
	}
}

func TestApplyAndOOBAttrs(t *testing.T) {
	header := http.Header{}
	Apply(NewHTMX4(nil), header, Response{PushURL: "false"})

	if header.Get("HX-Push-Url") != "false" {
		t.Fatalf("Apply() headers = %v", header)
	}

	Apply(nil, header, Response{PushURL: "x"})

	if got := OOBAttrs(NewHTMX4(nil), "beforeend"); !reflect.DeepEqual(got, map[string]string{"hx-swap-oob": "beforeend"}) {
		t.Fatalf("htmx OOBAttrs() = %v", got)
	}

	if got := OOBAttrs(NewHTMX(nil), ""); !reflect.DeepEqual(got, map[string]string{"hx-swap-oob": "true"}) {
		t.Fatalf("htmx 2 OOBAttrs() = %v", got)
	}

	if got := OOBAttrs(NewUnpoly(nil), "beforeend"); !reflect.DeepEqual(got, map[string]string{"up-hungry": ""}) {
		t.Fatalf("unpoly OOBAttrs() = %v", got)
	}

	if got := OOBAttrs(NewTurbo(nil), "beforeend"); !reflect.DeepEqual(got, map[string]string{"hx-swap-oob": "beforeend"}) {
		t.Fatalf("turbo OOBAttrs() = %v", got)
	}
}

func TestHTMX4QuotesInheritedBoostValuesWithWhitespace(t *testing.T) {
	tests := []struct {
		name string
		attr Attr
		want string
	}{
		{"swap with a modifier", Attr{Swap: "outerHTML showTarget:body"}, "swap:'outerHTML showTarget:body'"},
		{"target with a space", Attr{Target: "closest form"}, "target:'closest form'"},
		{"target with a comma", Attr{Target: "#a,#b"}, "target:'#a,#b'"},
		{"value with a single quote", Attr{Target: "find button[name='go'] x"}, `target:"find button[name='go'] x"`},
		{"value already quoted", Attr{Swap: "'outerHTML showTarget:body'"}, "swap:'outerHTML showTarget:body'"},
		{"plain values", Attr{Target: "#content", Swap: "outerHTML"}, "target:#content swap:outerHTML"},
	}

	for _, test := range tests {
		attr := test.attr
		attr.Inherit = true
		attr.Boost = BoostOn

		got := Attrs(NewHTMX4(nil), attr)["hx-boost:inherited"]
		if got != test.want {
			t.Errorf("%s: hx-boost:inherited = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestReloadOnErrorIsTheSameForEveryConnector(t *testing.T) {
	for _, conn := range []Connector{NewHTMX4(nil), NewHTMX(nil), NewUnpoly(nil), NewTurbo(nil), NewPartial(nil)} {
		for _, inherit := range []bool{false, true} {
			attrs := Attrs(conn, Attr{ReloadOnError: true, Inherit: inherit})
			if value, ok := attrs[ReloadOnErrorAttr]; !ok || value != "" || len(attrs) != 1 {
				t.Errorf("%T inherit %v: Attrs() = %v, want only %s", conn, inherit, attrs, ReloadOnErrorAttr)
			}
		}
	}

	merged := MergeAttrs(NewHTMX4(nil), map[string]string{ReloadOnErrorAttr: "", "class": "x"}, Attr{}, AttrReloadOnError)
	if !reflect.DeepEqual(merged, map[string]string{"class": "x"}) {
		t.Fatalf("MergeAttrs(remove reload) = %v", merged)
	}
}
