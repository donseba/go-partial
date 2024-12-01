package connector

type Turbo struct {
	base
}

func NewTurbo(c *Config) Connector {
	return &Turbo{
		base: base{
			config:       c,
			targetHeader: "Turbo-Frame",
			selectHeader: "Turbo-Select",
			actionHeader: "Turbo-Action",
		},
	}
}
