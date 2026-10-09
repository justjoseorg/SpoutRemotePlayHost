package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
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
	if r := do(h, "GET", "/", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "SpoutRemotePlayHost") {
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

type fakeDisp struct {
	active bool
	modes  []display.Mode
}

func (f *fakeDisp) Create(m display.Mode) error {
	f.active = true
	f.modes = append(f.modes, m)
	return nil
}
func (f *fakeDisp) Destroy() error { f.active = false; return nil }
func (f *fakeDisp) Active() bool   { return f.active }

func TestModeChangeRecreatesActiveMonitor(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	fd := &fakeDisp{active: true}
	h := New(store, fd, "test", "tok")
	body := `{"width":1920,"height":1080,"refreshHz":120,"autoCreate":true,"codec":"auto"}`
	if rec := do(h, "PUT", "/api/config", body); rec.Code != 200 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if len(fd.modes) != 1 || fd.modes[0] != (display.Mode{Width: 1920, Height: 1080, RefreshHz: 120}) {
		t.Fatalf("expected recreate at 1920x1080@120, got %v", fd.modes)
	}
	do(h, "PUT", "/api/config", body)
	if len(fd.modes) != 1 {
		t.Fatalf("unchanged mode must not recreate, got %v", fd.modes)
	}
}
