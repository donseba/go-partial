// Package pageflow contains experimental helpers for multi-step page flows.
package pageflow

import (
	"net/http"

	partial "github.com/donseba/go-partial"
)

// Step describes a single page-flow step.
type Step struct {
	Name     string
	Partial  *partial.Partial
	Validate func(r *http.Request, data map[string]any) error
}

// PageFlow contains the ordered steps for a flow.
type PageFlow struct {
	Steps []Step
}

// SessionData stores per-session progress and data for a PageFlow.
type SessionData struct {
	StepData  map[string]map[string]any
	Validated map[string]bool
	Current   string
}

// New creates a PageFlow from an ordered set of steps.
func New(steps []Step) *PageFlow {
	return &PageFlow{Steps: steps}
}

// FirstStep returns the first configured step.
func (f *PageFlow) FirstStep() *Step {
	if len(f.Steps) == 0 {
		return nil
	}
	return &f.Steps[0]
}

// CurrentStep returns the active step for a session.
func (f *PageFlow) CurrentStep(session *SessionData) *Step {
	if session == nil || session.Current == "" {
		return f.FirstStep()
	}
	idx := f.FindStep(session.Current)
	if idx == -1 {
		return f.FirstStep()
	}
	return &f.Steps[idx]
}

// SetCurrentStep moves a session to a named step when it exists.
func (f *PageFlow) SetCurrentStep(session *SessionData, stepName string) bool {
	if session == nil || f.FindStep(stepName) == -1 {
		return false
	}
	session.Current = stepName
	return true
}

// Next moves a session to the next step.
func (f *PageFlow) Next(session *SessionData) bool {
	if session == nil {
		return false
	}
	current := f.CurrentStep(session)
	if current == nil {
		return false
	}
	idx := f.FindStep(current.Name)
	if idx >= 0 && idx < len(f.Steps)-1 {
		session.Current = f.Steps[idx+1].Name
		return true
	}
	return false
}

// Prev moves a session to the previous step.
func (f *PageFlow) Prev(session *SessionData) bool {
	if session == nil {
		return false
	}
	current := f.CurrentStep(session)
	if current == nil {
		return false
	}
	idx := f.FindStep(current.Name)
	if idx > 0 {
		session.Current = f.Steps[idx-1].Name
		return true
	}
	return false
}

// FindStep returns the index of a named step or -1 when absent.
func (f *PageFlow) FindStep(name string) int {
	for i, step := range f.Steps {
		if step.Name == name {
			return i
		}
	}
	return -1
}

// AllPreviousValidated reports whether every step before the current one passed validation.
func (f *PageFlow) AllPreviousValidated(session *SessionData) bool {
	if session == nil {
		return false
	}
	curStep := f.CurrentStep(session)
	if curStep == nil {
		return false
	}
	curIdx := f.FindStep(curStep.Name)
	for i := 0; i < curIdx; i++ {
		if !session.Validated[f.Steps[i].Name] {
			return false
		}
	}
	return true
}

// SetStepValidated stores the validation state for a step.
func (session *SessionData) SetStepValidated(stepName string, valid bool) {
	if session.Validated == nil {
		session.Validated = make(map[string]bool)
	}
	session.Validated[stepName] = valid
}

// SetStepData stores data for a step.
func (session *SessionData) SetStepData(stepName string, data map[string]any) {
	if session.StepData == nil {
		session.StepData = make(map[string]map[string]any)
	}
	session.StepData[stepName] = data
}

// GetStepData returns stored data for a step.
func (session *SessionData) GetStepData(stepName string) map[string]any {
	if session.StepData == nil {
		return nil
	}
	return session.StepData[stepName]
}

// GetAllData returns all step data merged into one map.
func (session *SessionData) GetAllData() map[string]any {
	merged := make(map[string]any)
	for _, data := range session.StepData {
		for k, v := range data {
			merged[k] = v
		}
	}
	return merged
}
