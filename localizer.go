package partial

import "context"

var (
	LocalizerContextKey = localizerContextKey{}
	LocalizerDefault    = &defaultLocalizer{locale: "en_US"}
)

type localizerContextKey struct{}

type Localizer interface {
	GetLocale() string
}

func getLocalizer(ctx context.Context) Localizer {
	if ctx == nil {
		return LocalizerDefault
	}
	if loc, ok := ctx.Value(LocalizerContextKey).(Localizer); ok {
		return loc
	}
	return LocalizerDefault
}

type defaultLocalizer struct {
	locale string
}

func (d *defaultLocalizer) GetLocale() string {
	return d.locale
}
