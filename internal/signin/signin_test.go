package signin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func do(h http.Handler, method, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/signin", strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func newTest(typ Typer) *Handler {
	return NewHandler(func(t string) bool { return t == "good" }, typ)
}

func TestNeedsPairedToken(t *testing.T) {
	typed := false
	h := newTest(func(string) error { typed = true; return nil })
	for _, tok := range []string{"", "bad"} {
		if w := do(h, "POST", tok, `{"pin":"1234"}`); w.Code != 401 {
			t.Fatalf("token %q: got %d", tok, w.Code)
		}
	}
	if typed {
		t.Fatal("typed without a valid token")
	}
	if w := do(h, "GET", "good", ""); w.Code != 200 {
		t.Fatalf("GET: %d", w.Code)
	}
}

func TestTypesValidPIN(t *testing.T) {
	var got string
	h := newTest(func(p string) error { got = p; return nil })
	if w := do(h, "POST", "good", `{"pin":"024680"}`); w.Code != 200 || got != "024680" {
		t.Fatalf("code %d, typed %q", w.Code, got)
	}
}

func TestRejectsBadPIN(t *testing.T) {
	h := newTest(func(string) error { t.Fatal("typed a bad PIN"); return nil })
	for _, pin := range []string{"", "123", "12a4", "1234\n", strings.Repeat("1", 33)} {
		if w := do(h, "POST", "good", `{"pin":"`+strings.ReplaceAll(pin, "\n", `\n`)+`"}`); w.Code != 400 {
			t.Fatalf("pin %q: got %d", pin, w.Code)
		}
	}
}

func TestNotSignInScreen(t *testing.T) {
	h := newTest(func(string) error { return ErrNotSignInScreen })
	if w := do(h, "POST", "good", `{"pin":"1234"}`); w.Code != 409 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestRateLimit(t *testing.T) {
	now := time.Unix(1000, 0)
	h := newTest(func(string) error { return nil })
	h.now = func() time.Time { return now }
	for i := 0; i < maxTries; i++ {
		if w := do(h, "POST", "good", `{"pin":"1234"}`); w.Code != 200 {
			t.Fatalf("try %d: %d", i, w.Code)
		}
	}
	if w := do(h, "POST", "good", `{"pin":"1234"}`); w.Code != 429 {
		t.Fatalf("over limit: %d", w.Code)
	}
	now = now.Add(window)
	if w := do(h, "POST", "good", `{"pin":"1234"}`); w.Code != 200 {
		t.Fatalf("after window: %d", w.Code)
	}
}

func TestOneAttemptAtATime(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h := newTest(func(string) error { close(started); <-release; return nil })
	done := make(chan int)
	go func() { done <- do(h, "POST", "good", `{"pin":"1234"}`).Code }()
	<-started
	if w := do(h, "POST", "good", `{"pin":"1234"}`); w.Code != 409 {
		t.Fatalf("concurrent attempt: %d", w.Code)
	}
	close(release)
	if c := <-done; c != 200 {
		t.Fatalf("first attempt: %d", c)
	}
}
