package partial

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageFlowBasicNavigationAndValidation(t *testing.T) {
	steps := []FlowStep{
		{Name: "info"},
		{
			Name: "form",
			Validate: func(r *http.Request, data map[string]any) error {
				if data["field"] == "ok" {
					return nil
				}
				return errors.New("invalid")
			},
		},
		{Name: "confirm"},
	}
	flow := NewPageFlow(steps)
	session := &FlowSessionData{}

	if !flow.AllPreviousValidated(session) {
		t.Error("first step should not require previous validation")
	}
	session.SetStepValidated("info", true)

	flow.Next(session)
	if !flow.AllPreviousValidated(session) {
		t.Error("second step should see first as validated")
	}

	badData := map[string]any{"field": "bad"}
	if err := steps[1].Validate(nil, badData); err == nil {
		t.Error("expected validation error for bad data")
	}
	session.SetStepValidated("form", false)

	goodData := map[string]any{"field": "ok"}
	if err := steps[1].Validate(nil, goodData); err != nil {
		t.Errorf("expected no error for good data, got: %v", err)
	}
	session.SetStepValidated("form", true)
	session.SetStepData("form", goodData)

	flow.Next(session)
	if !flow.AllPreviousValidated(session) {
		t.Error("third step should see previous as validated")
	}

	session.SetStepData("info", map[string]any{"foo": 1})
	all := session.GetAllData()
	if all["foo"] != 1 || all["field"] != "ok" {
		t.Errorf("aggregated data incorrect: %#v", all)
	}
}

func TestPageFlowStepFromURL(t *testing.T) {
	steps := []FlowStep{
		{Name: "one"},
		{Name: "two"},
		{Name: "three"},
	}
	flow := NewPageFlow(steps)
	session := &FlowSessionData{}

	for _, stepName := range []string{"two", "three"} {
		if !flow.SetCurrentStep(session, stepName) {
			t.Fatalf("step %q could not be set", stepName)
		}
		if flow.CurrentStep(session).Name != stepName {
			t.Errorf("expected current step to be %q, got %q", stepName, flow.CurrentStep(session).Name)
		}
	}

	if idx := flow.FindStep("invalid"); idx != -1 {
		t.Errorf("expected -1 for invalid step, got %d", idx)
	}

	if flow.SetCurrentStep(session, "invalid") {
		t.Error("invalid step should not be set")
	}
}

func TestPageFlowRenderIntegration(t *testing.T) {
	infoPartial := New().ID("info").SetData(map[string]any{"msg": "Welcome!"})
	formPartial := New().ID("form").SetData(map[string]any{"prompt": "Enter value:"})
	confirmPartial := New().ID("confirm").SetData(map[string]any{"done": true})

	steps := []FlowStep{
		{Name: "info", Partial: infoPartial},
		{
			Name:    "form",
			Partial: formPartial,
			Validate: func(r *http.Request, data map[string]any) error {
				if data["field"] == "ok" {
					return nil
				}
				return errors.New("invalid")
			},
		},
		{Name: "confirm", Partial: confirmPartial},
	}
	flow := NewPageFlow(steps)

	renderStep := func(stepName string) map[string]any {
		session := &FlowSessionData{}
		if !flow.SetCurrentStep(session, stepName) {
			t.Fatalf("step %q not found", stepName)
		}
		step := flow.CurrentStep(session)
		if step.Partial == nil {
			t.Fatalf("step %q has no partial", stepName)
		}
		return step.Partial.data
	}

	if got := renderStep("info")["msg"]; got != "Welcome!" {
		t.Errorf("expected info step msg, got %#v", got)
	}
	if got := renderStep("form")["prompt"]; got != "Enter value:" {
		t.Errorf("expected form prompt, got %#v", got)
	}
	if got := renderStep("confirm")["done"]; got != true {
		t.Errorf("expected confirm done, got %#v", got)
	}
}

func TestPageFlowEndToEndHTTP(t *testing.T) {
	infoPartial := New().ID("info").SetData(map[string]any{"msg": "Welcome!"})
	formPartial := New().ID("form").SetData(map[string]any{"prompt": "Enter value:"})
	confirmPartial := New().ID("confirm").SetData(map[string]any{"done": true})

	steps := []FlowStep{
		{Name: "info", Partial: infoPartial},
		{
			Name:    "form",
			Partial: formPartial,
			Validate: func(r *http.Request, data map[string]any) error {
				if data["field"] == "ok" {
					return nil
				}
				return errors.New("invalid")
			},
		},
		{Name: "confirm", Partial: confirmPartial},
	}
	flow := NewPageFlow(steps)

	sessionStore := map[string]*FlowSessionData{}
	sessionID := "testsession"

	getSession := func(r *http.Request) *FlowSessionData {
		s, ok := sessionStore[sessionID]
		if !ok {
			s = &FlowSessionData{}
			sessionStore[sessionID] = s
		}
		return s
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := getSession(r)
		stepName := r.URL.Query().Get("step")
		if stepName != "" {
			if !flow.SetCurrentStep(session, stepName) {
				http.Error(w, "step not found", http.StatusNotFound)
				return
			}
		}
		step := flow.CurrentStep(session)

		if r.Method == http.MethodPost && step.Validate != nil {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			data := map[string]any{"field": r.FormValue("field")}
			err := step.Validate(r, data)
			if err == nil {
				session.SetStepValidated(step.Name, true)
				session.SetStepData(step.Name, data)
				flow.SetCurrentStep(session, "confirm")
				http.Redirect(w, r, "/?step=confirm", http.StatusSeeOther)
				return
			}
			session.SetStepValidated(step.Name, false)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("validation failed"))
			return
		}

		w.Header().Set("Content-Type", "text/html")
		switch step.Name {
		case "info":
			_, _ = w.Write([]byte("<h1>" + step.Partial.data["msg"].(string) + "</h1>"))
		case "form":
			_, _ = w.Write([]byte("<form method='POST'><input name='field'><button>Submit</button></form>"))
		case "confirm":
			_, _ = w.Write([]byte("<div>Done: true</div>"))
		}
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "?step=info")
	if err != nil {
		t.Fatalf("GET info failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("error closing response body: %v", cerr)
	}
	if string(body) != "<h1>Welcome!</h1>" {
		t.Errorf("expected info page, got: %s", string(body))
	}

	resp, err = http.Get(ts.URL + "?step=form")
	if err != nil {
		t.Fatalf("GET form failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("error closing response body: %v", cerr)
	}
	if string(body) != "<form method='POST'><input name='field'><button>Submit</button></form>" {
		t.Errorf("expected form page, got: %s", string(body))
	}

	resp, err = http.PostForm(ts.URL+"?step=form", map[string][]string{"field": {"bad"}})
	if err != nil {
		t.Fatalf("POST form failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("error closing response body: %v", cerr)
	}
	if resp.StatusCode != http.StatusBadRequest || string(body) != "validation failed" {
		t.Errorf("expected validation failure, got: %d %s", resp.StatusCode, string(body))
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err = client.PostForm(ts.URL+"?step=form", map[string][]string{"field": {"ok"}})
	if err != nil {
		t.Fatalf("POST form valid failed: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("error closing response body: %v", cerr)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected redirect after valid form, got: %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "?step=confirm")
	if err != nil {
		t.Fatalf("GET confirm failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("error closing response body: %v", cerr)
	}
	if string(body) != "<div>Done: true</div>" {
		t.Errorf("expected confirm page, got: %s", string(body))
	}
}
