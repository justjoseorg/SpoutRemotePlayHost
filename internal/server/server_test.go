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
	return New(store, display.New(), "test"), path
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestConfigRoundTripAndPersist(t *testing.T) {
	h, path := setup(t)
	body := `{"width":1920,"height":1080,"refreshHz":120,"autoCreate":false}`
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
	for _, b := range []string{`{"width":1,"height":800,"refreshHz":60}`, `{"bogus":1}`, `nope`} {
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
