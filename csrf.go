package partial

import (
	"context"
	"fmt"
	"time"
)

const DefaultCsrfToken = "X-CSRF-Token"

var CsrfContextKey = csrfContextKey{}

type csrfContextKey struct{}

type CsrfToken interface {
	Token(ctx context.Context) string
	Key() string
}

func getCsrfToken(ctx context.Context) CsrfToken {
	if csrfer, ok := ctx.Value(CsrfContextKey).(CsrfToken); ok {
		return csrfer
	}

	timeToken := time.Now().UnixNano()

	return &defaultCsrf{
		token: fmt.Sprintf("invalid-token-%d", timeToken),
		key:   DefaultCsrfToken,
	}
}

type defaultCsrf struct {
	token string
	key   string
}

func (d *defaultCsrf) Token(ctx context.Context) string {
	if token, ok := ctx.Value(CsrfContextKey).(string); ok {
		return token
	}

	return d.token
}

func (d *defaultCsrf) Key() string {
	return d.key
}
