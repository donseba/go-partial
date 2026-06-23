package partial

import "testing"

func TestGetBasePathSimple(t *testing.T) {
	p := New()
	p.SetBasePath("/foo")
	if got := p.GetBasePath(); got != "/foo" {
		t.Errorf("expected /foo, got %q", got)
	}
}

func TestGetBasePathParentFallback(t *testing.T) {
	parent := New()
	parent.SetBasePath("/parent")
	child := New().SetParent(parent)
	if got := child.GetBasePath(); got != "/parent" {
		t.Errorf("expected /parent, got %q", got)
	}
}

func TestGetBasePathParentChain(t *testing.T) {
	grandparent := New()
	grandparent.SetBasePath("/grand")
	parent := New().SetParent(grandparent)
	child := New().SetParent(parent)
	if got := child.GetBasePath(); got != "/grand" {
		t.Errorf("expected /grand, got %q", got)
	}
}

func TestGetBasePathOverride(t *testing.T) {
	parent := New()
	parent.SetBasePath("/parent")
	child := New().SetParent(parent)
	child.SetBasePath("/child")
	if got := child.GetBasePath(); got != "/child" {
		t.Errorf("expected /child, got %q", got)
	}
}

func TestGetBasePathEmpty(t *testing.T) {
	p := New()
	if got := p.GetBasePath(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
