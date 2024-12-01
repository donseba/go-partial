package connector

type Partial struct {
	base
}

func NewPartial(c *Config) Connector {
	return &Partial{
		base: base{
			config:       c,
			targetHeader: "X-Target",
			selectHeader: "X-Select",
			actionHeader: "X-Action",
		},
	}
}
