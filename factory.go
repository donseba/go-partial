package partial

// Factory creates request-scoped partials from a configured prototype. The
// returned values are ordinary *Partial instances and can use the complete
// native API directly.
type Factory struct {
	prototype *Partial
}

// NewFactory creates a factory from prototype. Later changes to prototype do
// not affect the factory.
func NewFactory(prototype *Partial) *Factory {
	if prototype == nil {
		prototype = New()
	}
	return &Factory{prototype: prototype.Clone()}
}

// New creates a partial with the factory configuration and template paths.
func (f *Factory) New(templates ...string) *Partial {
	if f == nil || f.prototype == nil {
		return New(templates...)
	}
	return f.prototype.Clone().SetTemplates(templates...)
}

// NewID creates a named partial with the factory configuration and template
// paths.
func (f *Factory) NewID(id string, templates ...string) *Partial {
	return f.New(templates...).ID(id)
}
