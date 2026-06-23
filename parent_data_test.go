package partial

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParentDataInjection(t *testing.T) {
	svc := NewService(&Config{})

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/parent.html": `<div>Parent: {{ .Data.ParentName }} {{ child "child" }}</div>`,
			"templates/child.html":  `<div>Child: {{ .Data.ChildName }}, Parent's Name: {{ .Parent.ParentName }}</div>`,
		},
	}

	childPartial := New("templates/child.html").ID("child")
	childPartial.SetData(map[string]any{
		"ChildName": "Child1",
	})

	parentPartial := New("templates/parent.html").ID("parent")
	parentPartial.SetData(map[string]any{
		"ParentName": "Parent1",
	})
	parentPartial.With(childPartial)

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	out, err := svc.NewLayout().FS(fsys).Set(parentPartial).RenderWithRequest(request.Context(), request)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	_, _ = response.Write([]byte(out))

	expected := `<div>Parent: Parent1 <div>Child: Child1, Parent's Name: Parent1</div></div>`
	if strings.TrimSpace(response.Body.String()) != expected {
		t.Errorf("expected %s, got %s", expected, response.Body.String())
	}
}

func TestParentDataWithNestedChildren(t *testing.T) {
	svc := NewService(&Config{})

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/grandparent.html": `<div>GP: {{ .Data.Name }} {{ child "parent" }}</div>`,
			"templates/parent.html":      `<div>P: {{ .Data.Name }}, GP: {{ .Parent.Name }} {{ child "child" }}</div>`,
			"templates/child.html":       `<div>C: {{ .Data.Name }}, P: {{ .Parent.Name }}</div>`,
		},
	}

	childPartial := New("templates/child.html").ID("child")
	childPartial.SetData(map[string]any{
		"Name": "Child",
	})

	parentPartial := New("templates/parent.html").ID("parent")
	parentPartial.SetData(map[string]any{
		"Name": "Parent",
	})
	parentPartial.With(childPartial)

	grandparentPartial := New("templates/grandparent.html").ID("grandparent")
	grandparentPartial.SetData(map[string]any{
		"Name": "Grandparent",
	})
	grandparentPartial.With(parentPartial)

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	out, err := svc.NewLayout().FS(fsys).Set(grandparentPartial).RenderWithRequest(request.Context(), request)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	expected := `<div>GP: Grandparent <div>P: Parent, GP: Grandparent <div>C: Child, P: Parent</div></div></div>`
	if strings.TrimSpace(string(out)) != expected {
		t.Errorf("expected %s, got %s", expected, out)
	}
}

func TestParentDataWithChildFunc(t *testing.T) {
	svc := NewService(&Config{})

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/parent.html": `<div>{{ .Data.Title }} {{ child "item" (dict "ItemID" "123") }}</div>`,
			"templates/item.html":   `<div>Item: {{ .Data.ItemID }}, Title: {{ .Parent.Title }}</div>`,
		},
	}

	itemPartial := New("templates/item.html").ID("item")

	parentPartial := New("templates/parent.html").ID("parent")
	parentPartial.SetData(map[string]any{
		"Title": "My List",
	})
	parentPartial.With(itemPartial)

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	out, err := svc.NewLayout().FS(fsys).Set(parentPartial).RenderWithRequest(request.Context(), request)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	expected := `<div>My List <div>Item: 123, Title: My List</div></div>`
	if strings.TrimSpace(string(out)) != expected {
		t.Errorf("expected %s, got %s", expected, out)
	}
}

func TestParentDataIsNilWhenNoParent(t *testing.T) {
	svc := NewService(&Config{})

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/root.html": `<div>Root{{ if .Parent }}, Parent: {{ .Parent.Name }}{{ end }}</div>`,
		},
	}

	rootPartial := New("templates/root.html").ID("root")
	rootPartial.SetData(map[string]any{
		"Name": "Root",
	})

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	out, err := svc.NewLayout().FS(fsys).Set(rootPartial).RenderWithRequest(request.Context(), request)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	expected := `<div>Root</div>`
	if strings.TrimSpace(string(out)) != expected {
		t.Errorf("expected %s, got %s", expected, out)
	}
}
