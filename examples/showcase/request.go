package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
)

func (app *App) requestContext(r *http.Request) context.Context {
	ctx := context.WithValue(r.Context(), partial.LocalizerContextKey, showcaseLocalizer{locale: app.localeFromRequest(r)})
	return context.WithValue(ctx, partial.CsrfContextKey, showcaseCsrf{
		key:   partial.DefaultCsrfToken,
		token: randomID(),
	})
}

func (app *App) flowSession(w http.ResponseWriter, r *http.Request) *partial.FlowSessionData {
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
		session = &partial.FlowSessionData{}
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
