package actions_test

import (
	"html/template"
	"slices"
	"testing"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/exp/actions"
)

// TestFuncMapStandsInForTheStage registers FuncMap next to Stage from
// another package, as an application does. Where the compiler inlines
// Stage into this test, the functions of the stage are named after this
// package; its stand-ins still raise no func.protected warning.
func TestFuncMapStandsInForTheStage(t *testing.T) {
	var warned []string

	sink := partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
		if event.Kind == partial.EventFuncProtected {
			warned = append(warned, event.Fields["function"].(string))
		}
	})

	partial.New().SetEvents(sink).Use(actions.Stage()).SetFunc(actions.FuncMap())
	partial.New().SetEvents(sink).SetFunc(actions.FuncMap()).Use(actions.Stage())

	if len(warned) > 0 {
		t.Fatalf("warned about the stage's own stand-ins %v", warned)
	}

	// A function of the application's is never called: the stage gives
	// templates the name.
	own := template.FuncMap{
		"action": func() template.HTML {
			return ""
		},
	}

	partial.New().SetEvents(sink).Use(actions.Stage()).SetFunc(own)

	if !slices.Equal(warned, []string{"action"}) {
		t.Fatalf("warned about %v, want action", warned)
	}
}
