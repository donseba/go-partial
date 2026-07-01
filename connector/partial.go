package connector

type Partial struct {
	base
}

func NewPartial(c *Config) Connector {
	return &Partial{
		base: base{
			config:       c,
			targetHeader: HeaderTarget.String(),
			selectHeader: HeaderSelect.String(),
			actionHeader: HeaderAction.String(),
		},
	}
}
