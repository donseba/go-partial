package pageflow

import "testing"

// TestPrevWalksBackToTheFirstStep checks that Prev moves to the step before
// the current one, stops at the first, and refuses without a session or
// steps.
func TestPrevWalksBackToTheFirstStep(t *testing.T) {
	flow := New([]Step{{Name: "info"}, {Name: "form"}, {Name: "confirm"}})
	session := &SessionData{Current: "confirm"}

	if !flow.Prev(session) || session.Current != "form" {
		t.Fatalf("Prev() from confirm went to %q", session.Current)
	}

	if !flow.Prev(session) || session.Current != "info" {
		t.Fatalf("Prev() from form went to %q", session.Current)
	}

	if flow.Prev(session) || session.Current != "info" {
		t.Fatalf("Prev() from the first step went to %q", session.Current)
	}

	if flow.Prev(nil) {
		t.Fatal("Prev(nil) = true")
	}

	empty := New(nil)
	if empty.Prev(&SessionData{}) || empty.Next(&SessionData{}) || empty.Next(nil) {
		t.Fatal("a flow without steps moved")
	}

	if empty.FirstStep() != nil || empty.CurrentStep(nil) != nil {
		t.Fatal("a flow without steps has a step")
	}
}

// TestNextStopsAtTheLastStep checks that Next does not move past the last
// step, and that an unknown current step counts as the first.
func TestNextStopsAtTheLastStep(t *testing.T) {
	flow := New([]Step{{Name: "info"}, {Name: "confirm"}})

	last := &SessionData{Current: "confirm"}
	if flow.Next(last) || last.Current != "confirm" {
		t.Fatalf("Next() from the last step went to %q", last.Current)
	}

	unknown := &SessionData{Current: "gone"}
	if step := flow.CurrentStep(unknown); step == nil || step.Name != "info" {
		t.Fatalf("CurrentStep() of an unknown step = %+v, want the first", step)
	}

	if !flow.Next(unknown) || unknown.Current != "confirm" {
		t.Fatalf("Next() from an unknown step went to %q", unknown.Current)
	}
}

func TestAllPreviousValidatedNeedsEveryEarlierStep(t *testing.T) {
	flow := New([]Step{{Name: "info"}, {Name: "form"}, {Name: "confirm"}})
	session := &SessionData{Current: "confirm"}
	session.SetStepValidated("info", true)

	if flow.AllPreviousValidated(session) {
		t.Fatal("AllPreviousValidated() = true with form not validated")
	}

	session.SetStepValidated("form", true)

	if !flow.AllPreviousValidated(session) {
		t.Fatal("AllPreviousValidated() = false with every earlier step validated")
	}

	if flow.AllPreviousValidated(nil) || New(nil).AllPreviousValidated(&SessionData{}) {
		t.Fatal("AllPreviousValidated() = true without a session or steps")
	}
}

func TestStepDataOfAnEmptySession(t *testing.T) {
	session := &SessionData{}

	if session.GetStepData("info") != nil {
		t.Fatal("GetStepData() of an empty session is not nil")
	}

	if got := session.GetAllData(); len(got) != 0 {
		t.Fatalf("GetAllData() of an empty session = %v", got)
	}

	session.SetStepData("info", map[string]any{"name": "Ada"})
	session.SetStepData("form", map[string]any{"age": 36})

	if got := session.GetStepData("info")["name"]; got != "Ada" {
		t.Fatalf("GetStepData(info) name = %v", got)
	}

	if got := session.GetAllData(); got["name"] != "Ada" || got["age"] != 36 {
		t.Fatalf("GetAllData() = %v", got)
	}
}
