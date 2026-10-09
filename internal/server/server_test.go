package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
)

func setup(t *testing.T) (http.Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.json")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return New(store, display.New(), "test", "tok", mustPair(t)), path
}

func mustPair(t *testing.T) *pairing.Manager {
	t.Helper()
	m, err := pairing.New("", nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
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
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := New(store, errDisp{}, "test", "tok", mustPair(t))
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
	h := New(store, fd, "test", "tok", mustPair(t))
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

func doFrom(h http.Handler, addr, method, path, body, auth string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = addr
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func TestPairingFlow(t *testing.T) {
	h, _ := setup(t)
	const remote = "192.168.1.50:5555"
	salt, pin, secret := "saltsalt1234", "0427", "client-secret"
	reqBody := `{"name":"Deck","salt":"` + salt + `","pinHash":"` + pairing.PINHash(salt, pin) +
		`","secretHash":"` + pairing.SecretHash(secret) + `"}`

	rec := doFrom(h, remote, "POST", "/api/pair/request", reqBody, "")
	if rec.Code != 200 {
		t.Fatalf("request: %d %s", rec.Code, rec.Body)
	}
	id := strings.Split(strings.Split(rec.Body.String(), `"id":"`)[1], `"`)[0]
	poll := `{"id":"` + id + `","secret":"` + secret + `"}`

	if rec := doFrom(h, remote, "POST", "/api/pair/poll", poll, ""); !strings.Contains(rec.Body.String(), `"pending"`) {
		t.Fatalf("expected pending: %s", rec.Body)
	}
	if rec := doFrom(h, remote, "POST", "/api/pair/poll", `{"id":"`+id+`","secret":"nope"}`, ""); !strings.Contains(rec.Body.String(), `"gone"`) {
		t.Fatalf("wrong secret must not see the request: %s", rec.Body)
	}
	confirm := func(p string) int {
		return do(h, "POST", "/api/pair/confirm", `{"id":"`+id+`","pin":"`+p+`"}`).Code
	}
	if rec := doFrom(h, remote, "POST", "/api/pair/confirm", `{"id":"`+id+`","pin":"`+pin+`"}`, "Bearer tok"); rec.Code != 403 {
		t.Fatalf("remote confirm must be rejected even with the API token, got %d", rec.Code)
	}
	if c := confirm("1111"); c != 403 {
		t.Fatalf("wrong pin: %d", c)
	}
	if c := confirm(pin); c != 200 {
		t.Fatalf("right pin: %d", c)
	}
	rec = doFrom(h, remote, "POST", "/api/pair/poll", poll, "")
	if !strings.Contains(rec.Body.String(), `"approved"`) || !strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("expected approval with token: %s", rec.Body)
	}
	tok := strings.Split(strings.Split(rec.Body.String(), `"token":"`)[1], `"`)[0]
	if rec := doFrom(h, remote, "POST", "/api/pair/poll", poll, ""); !strings.Contains(rec.Body.String(), `"gone"`) {
		t.Fatalf("token must be handed out once: %s", rec.Body)
	}

	if rec := doFrom(h, remote, "GET", "/api/config", "", ""); rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := doFrom(h, remote, "GET", "/api/config", "", "Bearer "+tok); rec.Code != 200 {
		t.Fatalf("paired token rejected: %d", rec.Code)
	}
	if rec := doFrom(h, remote, "GET", "/api/clients", "", "Bearer "+tok); rec.Code != 403 {
		t.Fatalf("clients list is local-only: %d", rec.Code)
	}

	list := do(h, "GET", "/api/clients", "")
	if strings.Contains(list.Body.String(), "tokenHash") || !strings.Contains(list.Body.String(), "Deck") {
		t.Fatalf("bad client list: %s", list.Body)
	}
	cid := strings.Split(strings.Split(list.Body.String(), `"id":"`)[1], `"`)[0]
	if c := do(h, "DELETE", "/api/clients/"+cid, "").Code; c != 200 {
		t.Fatalf("revoke: %d", c)
	}
	if rec := doFrom(h, remote, "GET", "/api/config", "", "Bearer "+tok); rec.Code != 401 {
		t.Fatalf("revoked token still works: %d", rec.Code)
	}
}

func TestPairingLockoutAfterWrongPINs(t *testing.T) {
	h, _ := setup(t)
	salt := "saltsalt1234"
	body := `{"name":"x","salt":"` + salt + `","pinHash":"` + pairing.PINHash(salt, "0001") +
		`","secretHash":"` + pairing.SecretHash("s") + `"}`
	rec := doFrom(h, "10.0.0.2:1", "POST", "/api/pair/request", body, "")
	id := strings.Split(strings.Split(rec.Body.String(), `"id":"`)[1], `"`)[0]
	for i := 0; i < 5; i++ {
		do(h, "POST", "/api/pair/confirm", `{"id":"`+id+`","pin":"9999"}`)
	}
	if c := do(h, "POST", "/api/pair/confirm", `{"id":"`+id+`","pin":"0001"}`).Code; c != 404 {
		t.Fatalf("request should be cancelled after 5 wrong PINs, got %d", c)
	}
}

func TestDiscoverIsPublicAndMinimal(t *testing.T) {
	h, _ := setup(t)
	rec := doFrom(h, "192.168.1.9:1", "GET", "/api/discover", "", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"app":"spout-host"`) {
		t.Fatalf("discover: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "tok") {
		t.Fatal("discover must not leak secrets")
	}
	if rec := doFrom(h, "192.168.1.9:1", "GET", "/api/config", "", ""); rec.Code != 401 {
		t.Fatalf("config must stay protected: %d", rec.Code)
	}
}

func TestPerDeviceMonitorConfig(t *testing.T) {
	h, _ := setup(t)
	const remote = "192.168.1.50:5555"
	salt, pin, secret := "saltsalt1234", "0427", "client-secret"
	reqBody := `{"name":"Deck","salt":"` + salt + `","pinHash":"` + pairing.PINHash(salt, pin) +
		`","secretHash":"` + pairing.SecretHash(secret) + `"}`
	var rq struct{ ID string }
	_ = json.Unmarshal(doFrom(h, remote, "POST", "/api/pair/request", reqBody, "").Body.Bytes(), &rq)
	doFrom(h, "127.0.0.1:1", "POST", "/api/pair/confirm", `{"id":"`+rq.ID+`","pin":"`+pin+`"}`, "")
	var pr struct{ Token string }
	_ = json.Unmarshal(doFrom(h, remote, "POST", "/api/pair/poll", `{"id":"`+rq.ID+`","secret":"`+secret+`"}`, "").Body.Bytes(), &pr)
	auth := "Bearer " + pr.Token

	body := `{"width":1920,"height":1080,"refreshHz":120,"autoCreate":true,"codec":"auto"}`
	if rec := doFrom(h, remote, "PUT", "/api/config", body, auth); rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(doFrom(h, remote, "GET", "/api/config", "", auth).Body.String(), `"width":1920`) {
		t.Fatal("device should read back its own config")
	}
	if !strings.Contains(doFrom(h, "127.0.0.1:1", "GET", "/api/config", "", "").Body.String(), `"width":1920`) {
		t.Fatal("default config must be unchanged by a device")
	}
}

func TestLocalUIManagesDeviceConfig(t *testing.T) {
	h, _ := setup(t)
	const remote = "192.168.1.50:5555"
	salt, pin, secret := "saltsalt1234", "0427", "client-secret"
	reqBody := `{"name":"Deck","salt":"` + salt + `","pinHash":"` + pairing.PINHash(salt, pin) +
		`","secretHash":"` + pairing.SecretHash(secret) + `"}`
	var rq struct{ ID string }
	_ = json.Unmarshal(doFrom(h, remote, "POST", "/api/pair/request", reqBody, "").Body.Bytes(), &rq)
	doFrom(h, "127.0.0.1:1", "POST", "/api/pair/confirm", `{"id":"`+rq.ID+`","pin":"`+pin+`"}`, "")
	var cl []struct{ ID string }
	_ = json.Unmarshal(doFrom(h, "127.0.0.1:1", "GET", "/api/clients", "", "").Body.Bytes(), &cl)
	if len(cl) != 1 {
		t.Fatalf("clients: %v", cl)
	}
	u := "/api/clients/" + cl[0].ID + "/config"
	if b := doFrom(h, "127.0.0.1:1", "GET", u, "", "").Body.String(); !strings.Contains(b, `"custom":false`) {
		t.Fatalf("should inherit defaults: %s", b)
	}
	body := `{"width":1920,"height":1080,"refreshHz":120,"autoCreate":true,"codec":"hevc"}`
	if rec := doFrom(h, "127.0.0.1:1", "PUT", u, body, ""); rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	if b := doFrom(h, "127.0.0.1:1", "GET", u, "", "").Body.String(); !strings.Contains(b, `"custom":true`) || !strings.Contains(b, `"width":1920`) {
		t.Fatalf("custom not stored: %s", b)
	}
	if b := doFrom(h, "127.0.0.1:1", "DELETE", u, "", "").Body.String(); !strings.Contains(b, `"custom":false`) || !strings.Contains(b, `"width":1920`) {
		t.Fatalf("reset failed: %s", b)
	}
	if rec := doFrom(h, remote, "GET", u, "", "Bearer nope"); rec.Code != 401 {
		t.Fatalf("remote must not reach it: %d", rec.Code)
	}
	if rec := doFrom(h, "127.0.0.1:1", "GET", "/api/clients/nope/config", "", ""); rec.Code != 404 {
		t.Fatalf("unknown id: %d", rec.Code)
	}
}

func TestPairingStoresClientCapabilities(t *testing.T) {
	h, _ := setup(t)
	salt, pin := "saltsalt1234", "0427"
	body := `{"name":"Deck","salt":"` + salt + `","pinHash":"` + pairing.PINHash(salt, pin) +
		`","secretHash":"` + pairing.SecretHash("s") + `","caps":{"hevc":true,"av1":false,"width":1920,"height":1200,"refreshHz":120}}`
	var rq struct{ ID string }
	_ = json.Unmarshal(doFrom(h, "192.168.1.50:5555", "POST", "/api/pair/request", body, "").Body.Bytes(), &rq)
	doFrom(h, "127.0.0.1:1", "POST", "/api/pair/confirm", `{"id":"`+rq.ID+`","pin":"`+pin+`"}`, "")
	out := doFrom(h, "127.0.0.1:1", "GET", "/api/clients", "", "").Body.String()
	if !strings.Contains(out, `"hevc":true`) || !strings.Contains(out, `"width":1920`) || strings.Contains(out, "tokenHash") {
		t.Fatalf("caps missing or token leaked: %s", out)
	}
}

type errDisp struct{}

func (errDisp) Create(display.Mode) error { return display.ErrNotImplemented }
func (errDisp) Destroy() error            { return nil }
func (errDisp) Active() bool              { return false }

func TestSessionStartStop(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	fd := &fakeDisp{}
	srv := NewServer(store, fd, "test", "tok", mustPair(t))
	srv.Sessions().Grace = 0
	h := srv.Handler()
	if r := do(h, "POST", "/api/session", `{"state":"start"}`); r.Code != 200 || !fd.active || len(fd.modes) != 1 {
		t.Fatalf("start: %d active=%v", r.Code, fd.active)
	}
	if r := do(h, "POST", "/api/session", `{"state":"stop"}`); r.Code != 200 || fd.active {
		t.Fatalf("stop: %d active=%v", r.Code, fd.active)
	}
	if r := do(h, "POST", "/api/session", `{"state":"nope"}`); r.Code != 400 {
		t.Fatalf("bad state: %d", r.Code)
	}
}
