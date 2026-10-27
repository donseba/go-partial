package flash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Cookie keeps flash messages across a redirect, such as the "Saved"
// message after a form posts and the browser follows to another page:
//
//	cookie := flash.Cookie{Key: key}
//
//	cookie.Persist(w, flash.Success("Saved"))
//	http.Redirect(w, r, "/items", http.StatusSeeOther)
//
// and, for every request, in middleware or before rendering:
//
//	r = cookie.Restore(w, r)
//
// Restore moves the messages into the request's flash store, where
// templates show them with {{ flash }}, and clears the cookie, so they show
// once. Messages keep their level.
//
// The zero Cookie works; its fields change the cookie. Without a Key the
// cookie is not signed: anyone who can set cookies for the site can make it
// show a message, escaped like any other text. Set Key to accept only
// messages the application persisted.
type Cookie struct {
	// Name is the cookie name; "flash" when empty.
	Name string
	// Path is the cookie path; "/" when empty.
	Path string
	// Domain is the cookie domain; the request's host when empty.
	Domain string
	// MaxAge is how long the messages wait for the next request; a minute
	// when zero.
	MaxAge time.Duration
	// Secure sends the cookie over HTTPS only.
	Secure bool
	// SameSite is the cookie's same-site mode; http.SameSiteLaxMode when
	// zero, so the messages survive a redirect from another site.
	SameSite http.SameSite
	// Key signs the cookie with HMAC-SHA256; Restore ignores cookies with
	// another signature.
	Key []byte
	// MaxBytes caps the cookie value; 3072 when zero, below the 4096 bytes
	// browsers keep. Messages that do not fit are dropped, the last first,
	// and long texts are shortened.
	MaxBytes int
}

// persisted is a message as it is kept in the cookie.
type persisted struct {
	Level Level  `json:"l"`
	Text  string `json:"t"`
}

const (
	defaultCookieName     = "flash"
	defaultCookieMaxAge   = time.Minute
	defaultCookieMaxBytes = 3072
	signatureSeparator    = "."
)

// Persist keeps messages for the next request, replacing any persisted
// earlier in this response. Without messages it clears the cookie.
func (c Cookie) Persist(w http.ResponseWriter, messages ...Message) {
	if w == nil {
		return
	}

	value := c.encode(messages)
	if value == "" {
		http.SetCookie(w, c.cookie("", -1))
		return
	}

	http.SetCookie(w, c.cookie(value, int(c.maxAge()/time.Second)))
}

// Restore moves the messages persisted for this request into its flash
// store, after the messages the store already has, and clears the cookie.
// It returns the request with the store in its context; a request without
// persisted messages is returned as it is.
func (c Cookie) Restore(w http.ResponseWriter, r *http.Request) *http.Request {
	messages := c.Read(r)
	if r == nil {
		return r
	}

	if _, err := r.Cookie(c.name()); err == nil && w != nil {
		http.SetCookie(w, c.cookie("", -1))
	}

	if len(messages) == 0 {
		return r
	}

	ctx := r.Context()
	if store := FromContext(ctx); store != nil {
		store.Add(messages...)
		return r
	}

	return r.WithContext(WithStore(ctx, NewStore(messages...)))
}

// Read returns the messages persisted for the request without clearing
// them.
func (c Cookie) Read(r *http.Request) []Message {
	if r == nil {
		return nil
	}

	cookie, err := r.Cookie(c.name())
	if err != nil {
		return nil
	}

	return c.decode(cookie.Value)
}

func (c Cookie) encode(messages []Message) string {
	kept := make([]persisted, 0, len(messages))
	for _, message := range messages {
		message = New(message.Level, message.Text)
		if strings.TrimSpace(message.Text) == "" {
			continue
		}

		kept = append(kept, persisted(message))
	}

	limit := c.maxBytes()

	for len(kept) > 0 {
		value := c.seal(kept)
		if len(value) <= limit {
			return value
		}

		// Shorten a long last message before dropping it.
		last := &kept[len(kept)-1]
		if len(last.Text) > 64 {
			last.Text = truncate(last.Text, len(last.Text)/2)
			continue
		}

		kept = kept[:len(kept)-1]
	}

	return ""
}

func (c Cookie) seal(messages []persisted) string {
	data, err := json.Marshal(messages)
	if err != nil {
		return ""
	}

	value := base64.RawURLEncoding.EncodeToString(data)
	if len(c.Key) == 0 {
		return value
	}

	return value + signatureSeparator + c.sign(value)
}

func (c Cookie) decode(value string) []Message {
	if len(c.Key) > 0 {
		payload, signature, ok := strings.Cut(value, signatureSeparator)
		if !ok || !hmac.Equal([]byte(signature), []byte(c.sign(payload))) {
			return nil
		}

		value = payload
	}

	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil
	}

	var stored []persisted
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil
	}

	messages := make([]Message, 0, len(stored))
	for _, message := range stored {
		if strings.TrimSpace(message.Text) == "" {
			continue
		}

		messages = append(messages, New(message.Level, message.Text))
	}

	return messages
}

func (c Cookie) sign(value string) string {
	mac := hmac.New(sha256.New, c.Key)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c Cookie) cookie(value string, maxAge int) *http.Cookie {
	sameSite := c.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteLaxMode
	}

	path := c.Path
	if path == "" {
		path = "/"
	}

	return &http.Cookie{
		Name:     c.name(),
		Value:    value,
		Path:     path,
		Domain:   c.Domain,
		MaxAge:   maxAge,
		Secure:   c.Secure,
		HttpOnly: true,
		SameSite: sameSite,
	}
}

func (c Cookie) name() string {
	if c.Name == "" {
		return defaultCookieName
	}

	return c.Name
}

func (c Cookie) maxAge() time.Duration {
	if c.MaxAge <= 0 {
		return defaultCookieMaxAge
	}

	return c.MaxAge
}

func (c Cookie) maxBytes() int {
	if c.MaxBytes <= 0 {
		return defaultCookieMaxBytes
	}

	return c.MaxBytes
}

// truncate cuts text to at most limit bytes without splitting a character.
func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}

	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}

	return text
}
