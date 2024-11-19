package partial

import (
	"testing"
)

func TestTree(t *testing.T) {
	p := New("template1", "template2").ID("root")
	child := New("template1", "template2").ID("id")
	oobChild := New("template1", "template2").ID("id1")

	child.With(oobChild)

	p.With(child)
	p.WithOOB(oobChild)

	tr := Tree(p)

	if tr.ID != "root" {
		t.Errorf("expected root id to be root, got %s", tr.ID)
	}

	if tr.Nodes == nil {
		t.Errorf("expected nodes to be non-nil")
	}

	if len(tr.Nodes) != 2 {
		t.Errorf("expected 2 node, got %d", len(tr.Nodes))
	}

	if tr.Nodes[0].ID != "id" {
		t.Errorf("expected id to be id∂, got %s", tr.Nodes[0].ID)
	}

	if tr.Nodes[1].ID != "id1" {
		t.Errorf("expected id to be id1, got %s", tr.Nodes[1].ID)
	}

	if tr.Nodes[0].Nodes == nil {
		t.Errorf("expected nodes to be non-nil")
	}

	if len(tr.Nodes[0].Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(tr.Nodes[0].Nodes))
	}

	if tr.Nodes[0].Nodes[0].ID != "id1" {
		t.Errorf("expected id to be id1, got %s", tr.Nodes[0].Nodes[0].ID)
	}
}
