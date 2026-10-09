package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjoseorg/SpigotRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpigotRemotePlayHost/internal/display"
)

func setup(t *testing.T) (http.Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.json")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return New(store, display.New(), "test", "tok"), path
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:4000"
	h.ServeHTTP(rec, req)
	return rec
}

func TestConfigRoundTripAndPersist(t *testing.T) {
	h, path := setup(t)
	body := `{"width":1920,"height":1080,"refreshHz":120,"autoCreate":false,"codec":"av1"}`
	if r := do(h, "PUT", "/api/config", body); r.Code != 200 {
		t.Fatalf("put: %d %s", r.Code, r.Body)
	}
	if r := do(h, "GET", "/api/config", ""); !strings.Contains(r.Body.String(), `"refreshHz":120`) {
		t.Fatalf("get: %s", r.Body)
	}
	s, err := config.Open(path)
	if err != nil || s.Get().Width != 1920 {
		t.Fatalf("not persisted: %v %+v", err, s.Get())
	}
}

func TestConfigRejectsInvalid(t *testing.T) {
	h, _ := setup(t)
	for _, b := range []string{`{"width":1,"height":800,"refreshHz":60,"codec":"auto"}`, `{"width":1280,"height":800,"refreshHz":60,"codec":"pyrowave"}`, `{"bogus":1}`, `nope`} {
		if r := do(h, "PUT", "/api/config", b); r.Code != 400 {
			t.Errorf("%s: want 400 got %d", b, r.Code)
		}
	}
}

func TestCreateNotImplementedAndUI(t *testing.T) {
	h, _ := setup(t)
	if r := do(h, "POST", "/api/monitor/create", ""); r.Code != 501 {
		t.Errorf("create: want 501 got %d", r.Code)
	}
	if r := do(h, "GET", "/", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "SpigotRemotePlayHost") {
		t.Errorf("ui not served: %d", r.Code)
	}
}

func TestRemoteRequiresToken(t *testing.T) {
	h, _ := setup(t)
	req := func(auth string) int {
		r := httptest.NewRequest("GET", "/api/config", nil)
		r.RemoteAddr = "192.168.1.50:5555"
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	if c := req(""); c != 401 {
		t.Errorf("no token: %d", c)
	}
	if c := req("Bearer wrong"); c != 401 {
		t.Errorf("bad token: %d", c)
	}
	if c := req("Bearer tok"); c != 200 {
		t.Errorf("good token: %d", c)
	}
}

func TestCrossOriginWriteRejected(t *testing.T) {
	h, _ := setup(t)
	r := httptest.NewRequest("POST", "/api/monitor/destroy", nil)
	r.RemoteAddr = "127.0.0.1:4000"
	r.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}
