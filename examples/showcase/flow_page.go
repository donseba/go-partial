package main

import (
	"errors"
	"net/http"
	"strings"

	partial "github.com/donseba/go-partial"
)

func (app *App) flow(w http.ResponseWriter, r *http.Request) {
	session := app.flowSession(w, r)
	if session.Current == "" {
		session.Current = "account"
	}

	steps := app.flowSteps(session, "")
	flow := partial.NewPageFlow(steps)
	if flow.FindStep(session.Current) == -1 {
		session.Current = steps[0].Name
	}

	errorMessage := ""
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			errorMessage = err.Error()
		} else {
			switch r.FormValue("direction") {
			case "reset":
				*session = partial.FlowSessionData{Current: "account"}
			case "back":
				flow.Prev(session)
			default:
				step := flow.CurrentStep(session)
				data := flowFormData(r)
				if step != nil && step.Validate != nil {
					if err := step.Validate(r, data); err != nil {
						errorMessage = err.Error()
						session.SetStepValidated(step.Name, false)
						break
					}
				}
				if step != nil {
					session.SetStepValidated(step.Name, true)
					session.SetStepData(step.Name, data)
				}
				flow.Next(session)
			}
		}
	}

	steps = app.flowSteps(session, errorMessage)
	flow = partial.NewPageFlow(steps)
	app.renderPartial(w, r, app.flowPartial(flow, session, errorMessage))
}

func (app *App) flowSteps(session *partial.FlowSessionData, errorMessage string) []partial.FlowStep {
	email, _ := session.GetStepData("account")["email"].(string)
	name, _ := session.GetStepData("details")["name"].(string)
	plan, _ := session.GetStepData("details")["plan"].(string)
	account := partial.NewID("account", "templates/flow_account.gohtml").SetDot(FlowAccountPage{
		Email: email,
		Error: errorMessage,
	})
	details := partial.NewID("details", "templates/flow_details.gohtml").SetDot(FlowDetailsPage{
		Name:  name,
		Plan:  plan,
		Error: errorMessage,
	})
	confirm := partial.NewID("confirm", "templates/flow_confirm.gohtml").SetDot(FlowConfirmPage{
		AllData: session.GetAllData(),
	})

	return []partial.FlowStep{
		{
			Name:    "account",
			Partial: account,
			Validate: func(r *http.Request, data map[string]any) error {
				email, _ := data["email"].(string)
				if !strings.Contains(email, "@") {
					return errors.New("enter an email address before continuing")
				}
				return nil
			},
		},
		{
			Name:    "details",
			Partial: details,
			Validate: func(r *http.Request, data map[string]any) error {
				name, _ := data["name"].(string)
				if strings.TrimSpace(name) == "" {
					return errors.New("enter a project name before continuing")
				}
				return nil
			},
		},
		{Name: "confirm", Partial: confirm},
	}
}

func (app *App) flowPartial(flow *partial.PageFlow, session *partial.FlowSessionData, errorMessage string) *partial.Partial {
	current := flow.CurrentStep(session)
	currentName := ""
	if current != nil {
		currentName = current.Name
	}
	content := partial.NewID("content", "templates/flow.gohtml").SetDot(FlowPage{
		Title:       "Page flow",
		Steps:       flow.Steps,
		CurrentStep: currentName,
		Validated:   session.Validated,
		Error:       errorMessage,
	})
	for _, step := range flow.Steps {
		content.With(step.Partial)
	}
	return content
}

func flowFormData(r *http.Request) map[string]any {
	data := make(map[string]any)
	for key, values := range r.PostForm {
		if key == "direction" {
			continue
		}
		if len(values) > 0 {
			data[key] = values[0]
		}
	}
	return data
}
