package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/donseba/go-partial/exp/csrf"
	"github.com/donseba/go-partial/exp/localization"
	"github.com/donseba/go-partial/exp/metrics"
	"github.com/donseba/go-partial/exp/pageflow"
)

func (app *App) requestContext(r *http.Request) context.Context {
	ctx := localization.WithLocalizer(r.Context(), showcaseLocalizer{locale: app.localeFromRequest(r)})
	ctx = csrf.WithToken(ctx, showcaseCsrf{
		key:   csrf.DefaultTokenKey,
		token: randomID(),
	})
	requestID := requestIDFromRequest(r)
	traceID := traceIDFromRequest(r, requestID)
	ctx = metrics.WithRequestID(ctx, requestID)
	return metrics.WithTraceID(ctx, traceID)
}

func requestIDFromRequest(r *http.Request) string {
	if r == nil {
		return randomID()
	}
	if requestID := r.Header.Get(metrics.HeaderRequestID); requestID != "" {
		return requestID
	}
	return randomID()
}

func traceIDFromRequest(r *http.Request, fallback string) string {
	if r == nil {
		return fallback
	}
	if traceID := r.Header.Get(metrics.HeaderTraceID); traceID != "" {
		return traceID
	}
	return fallback
}

func (app *App) flowSession(w http.ResponseWriter, r *http.Request) *pageflow.SessionData {
	const cookieName = "go_partial_showcase_flow"
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		cookie = &http.Cookie{
			Name:     cookieName,
			Value:    randomID(),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)
	}
	session, ok := app.flowSessions[cookie.Value]
	if !ok {
		session = &pageflow.SessionData{}
		app.flowSessions[cookie.Value] = session
	}
	return session
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func (app *App) localeFromRequest(r *http.Request) string {
	switch r.URL.Query().Get("locale") {
	case "nl_NL":
		return "nl_NL"
	case "fr_FR":
		return "fr_FR"
	case "en_US":
		return "en_US"
	}

	acceptLanguage := strings.ToLower(r.Header.Get("Accept-Language"))
	if strings.HasPrefix(acceptLanguage, "nl") {
		return "nl_NL"
	}
	if strings.HasPrefix(acceptLanguage, "fr") {
		return "fr_FR"
	}
	return "en_US"
}
