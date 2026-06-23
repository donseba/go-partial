package connector

type Turbo struct {
	base
}

const (
	TurboHeaderTarget HeaderKey = "Turbo-Frame"
	TurboHeaderSelect HeaderKey = "Turbo-Select"
	TurboHeaderAction HeaderKey = "Turbo-Action"
)

func NewTurbo(c *Config) Connector {
	return &Turbo{
		base: base{
			config:       c,
			targetHeader: TurboHeaderTarget.String(),
			selectHeader: TurboHeaderSelect.String(),
			actionHeader: TurboHeaderAction.String(),
		},
	}
}
