package partial

import "testing"

func TestFactoryCreatesNativeConfiguredPartials(t *testing.T) {
	prototype := New("prototype.gohtml").ID("prototype").SetBasePath("/app").SetStatus(201)
	factory := NewFactory(prototype)

	created := factory.NewID("content", "content.gohtml")
	if created.PartialID() != "content" {
		t.Fatalf("id = %q", created.PartialID())
	}
	if got := created.TemplatePaths(); len(got) != 1 || got[0] != "content.gohtml" {
		t.Fatalf("templates = %#v", got)
	}
	if created.GetBasePath() != "/app" || created.getStatus() != 201 {
		t.Fatal("factory did not preserve prototype configuration")
	}

	prototype.SetStatus(204)
	if created.getStatus() != 201 {
		t.Fatal("factory retained mutable prototype state")
	}
}
