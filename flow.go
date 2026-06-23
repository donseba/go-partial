package partial

import "net/http"

// FlowStep represents a single step in a page flow.
type FlowStep struct {
	Name     string
	Partial  *Partial
	Validate func(r *http.Request, data map[string]any) error
}

// PageFlow manages a multi-step flow.
type PageFlow struct {
	Steps []FlowStep
}

// FlowSessionData holds all data and validation info for a flow, to be stored in session.
type FlowSessionData struct {
	StepData  map[string]map[string]any
	Validated map[string]bool
	Current   string
}

// NewPageFlow creates a new PageFlow with the given steps.
func NewPageFlow(steps []FlowStep) *PageFlow {
	return &PageFlow{
		Steps: steps,
	}
}

// FirstStep returns the first FlowStep.
func (f *PageFlow) FirstStep() *FlowStep {
	if len(f.Steps) == 0 {
		return nil
	}
	return &f.Steps[0]
}

// CurrentStep returns the FlowStep named by the session, or the first step when
// the session does not have a current step yet.
func (f *PageFlow) CurrentStep(session *FlowSessionData) *FlowStep {
	if session == nil || session.Current == "" {
		return f.FirstStep()
	}
	idx := f.FindStep(session.Current)
	if idx == -1 {
		return f.FirstStep()
	}
	return &f.Steps[idx]
}

// SetCurrentStep sets the current step in the session when the step exists.
func (f *PageFlow) SetCurrentStep(session *FlowSessionData, stepName string) bool {
	if session == nil || f.FindStep(stepName) == -1 {
		return false
	}
	session.Current = stepName
	return true
}

// Next advances to the next step if possible.
func (f *PageFlow) Next(session *FlowSessionData) bool {
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

// Prev goes back to the previous step if possible.
func (f *PageFlow) Prev(session *FlowSessionData) bool {
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

// FindStep returns the index of a step by name, or -1 if not found.
func (f *PageFlow) FindStep(name string) int {
	for i, step := range f.Steps {
		if step.Name == name {
			return i
		}
	}
	return -1
}

// AllPreviousValidated checks if all previous steps are validated.
func (f *PageFlow) AllPreviousValidated(session *FlowSessionData) bool {
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

// SetStepValidated marks a step as validated in the session.
func (session *FlowSessionData) SetStepValidated(stepName string, valid bool) {
	if session.Validated == nil {
		session.Validated = make(map[string]bool)
	}
	session.Validated[stepName] = valid
}

// SetStepData sets the data for a step in the session.
func (session *FlowSessionData) SetStepData(stepName string, data map[string]any) {
	if session.StepData == nil {
		session.StepData = make(map[string]map[string]any)
	}
	session.StepData[stepName] = data
}

// GetStepData gets the data for a step from the session.
func (session *FlowSessionData) GetStepData(stepName string) map[string]any {
	if session.StepData == nil {
		return nil
	}
	return session.StepData[stepName]
}

// GetAllData returns all step data as a single merged map.
func (session *FlowSessionData) GetAllData() map[string]any {
	merged := make(map[string]any)
	for _, data := range session.StepData {
		for k, v := range data {
			merged[k] = v
		}
	}
	return merged
}
