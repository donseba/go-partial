package flash

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

// follow returns the request a browser makes after rec's response.
func follow(rec *httptest.ResponseRecorder) *http.Request {
	next := httptest.NewRequest(http.MethodGet, "/items", nil)

	for _, cookie := range rec.Result().Cookies() {
		if cookie.MaxAge >= 0 {
			next.AddCookie(cookie)
		}
	}

	return next
}

func TestPersistAndRestoreKeepMessagesAcrossARedirect(t *testing.T) {
	rec := httptest.NewRecorder()
	Cookie{}.Persist(rec, Success("Saved"), Warn("Saved as a draft"), Info(" "))

	next := httptest.NewRecorder()
	r := Cookie{}.Restore(next, follow(rec))

	got := Messages(r.Context())
	if len(got) != 2 || got[0] != Success("Saved") || got[1] != Warn("Saved as a draft") {
		t.Fatalf("Messages() = %#v", got)
	}

	cleared := next.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != "flash" || cleared[0].MaxAge >= 0 {
		t.Fatalf("Restore() cookies = %v, want the flash cookie cleared", cleared)
	}
}

func TestRestoreAddsToAnExistingStore(t *testing.T) {
	rec := httptest.NewRecorder()
	Cookie{}.Persist(rec, Error("Failed"))

	r := follow(rec)
	r = r.WithContext(WithStore(r.Context(), NewStore(Info("Earlier"))))
	r = Cookie{}.Restore(httptest.NewRecorder(), r)

	if got := Messages(r.Context()); len(got) != 2 || got[1] != Error("Failed") {
		t.Fatalf("Messages() = %#v", got)
	}
}

func TestSignedCookieRejectsOtherValues(t *testing.T) {
	cookie := Cookie{Name: "notice", Key: []byte("secret")}

	rec := httptest.NewRecorder()
	cookie.Persist(rec, Success("Saved"))

	if got := cookie.Read(follow(rec)); len(got) != 1 {
		t.Fatalf("Read() = %v, want the signed message", got)
	}

	forged := httptest.NewRecorder()
	Cookie{Name: "notice"}.Persist(forged, Success("Forged"))

	if got := cookie.Read(follow(forged)); len(got) != 0 {
		t.Fatalf("Read() = %v, want an unsigned cookie ignored", got)
	}

	other := httptest.NewRecorder()
	Cookie{Name: "notice", Key: []byte("other")}.Persist(other, Success("Other"))

	if got := cookie.Read(follow(other)); len(got) != 0 {
		t.Fatalf("Read() = %v, want a cookie of another key ignored", got)
	}
}

func TestPersistKeepsTheCookieSmall(t *testing.T) {
	cookie := Cookie{MaxBytes: 300}

	rec := httptest.NewRecorder()
	cookie.Persist(rec, Success("Saved"), Info(strings.Repeat("é", 400)), Warn(strings.Repeat("w", 400)))

	value := rec.Result().Cookies()[0].Value
	if len(value) > 300 {
		t.Fatalf("cookie value has %d bytes, want at most 300", len(value))
	}

	got := cookie.Read(follow(rec))
	if len(got) == 0 || got[0] != Success("Saved") {
		t.Fatalf("Read() = %#v, want the first message kept", got)
	}

	for _, message := range got {
		if strings.ContainsRune(message.Text, '\uFFFD') {
			t.Fatalf("message %q was cut inside a character", message.Text)
		}
	}
}

func TestRestoredMessagesRender(t *testing.T) {
	rec := httptest.NewRecorder()
	Cookie{}.Persist(rec, Warn("Check the date"))
	r := Cookie{}.Restore(httptest.NewRecorder(), follow(rec))

	fsys := fstest.MapFS{"page.gohtml": &fstest.MapFile{Data: []byte(`{{ flash }}`)}}
	p := partial.NewID("page", "page.gohtml").SetFileSystem(fsys).SetFunc(FuncMap()).Use(Stage())

	out, err := partial.RenderWithRequest(r.Context(), r, p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "Check the date") || !strings.Contains(string(out), "warn") {
		t.Fatalf("output = %q", out)
	}
}
