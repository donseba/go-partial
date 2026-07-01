package connector

type Turbo struct {
	base
}

const (
	TurboHeaderTarget HeaderKey = "Turbo-Frame"
	TurboHeaderSelect HeaderKey = "Turbo-Select"
	TurboHeaderAction HeaderKey = "Turbo-Action"

	TurboAttrFrameID = "id"
	TurboAttrSource  = "src"
	TurboAttrLoading = "loading"
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

func (t *Turbo) InteractionAttrs(interaction Interaction) map[string]string {
	attrs := map[string]string{}
	switch interaction.Kind {
	case InteractionPrefetch:
		attrs["rel"] = "prefetch"
		attrs["href"] = interaction.URL
	case InteractionRefresh:
		attrs["data-turbo-refresh-url"] = interaction.URL
	case InteractionOn:
		attrs["data-partial-interaction"] = string(interaction.Kind)
		attrs["data-trigger"] = interaction.Trigger
		attrs["data-url"] = interaction.URL
		if interaction.Target != "" {
			attrs["data-target"] = interaction.Target
		}
	default:
		attrs[TurboAttrFrameID] = interaction.ID
		attrs[TurboAttrSource] = interaction.URL
		if interaction.Kind == InteractionAsync || interaction.Kind == InteractionReveal {
			attrs[TurboAttrLoading] = "lazy"
		}
	}
	return attrs
}
