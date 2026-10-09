package server

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/apps"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlib"
)

type fakeLib struct {
	ready bool
	next  uint32
	have  map[uint32]steamlib.Shortcut
	debug bool
}

func (f *fakeLib) Status() steamlib.Status {
	return steamlib.Status{Ready: f.ready, DebugEnabled: f.debug}
}
func (f *fakeLib) Add(s steamlib.Shortcut) (uint32, error) {
	if !f.ready {
		return 0, errors.New("steam not ready")
	}
	f.next++
	f.have[f.next] = s
	return f.next, nil
}
func (f *fakeLib) Remove(id uint32) error                       { delete(f.have, id); return nil }
func (f *fakeLib) Exists(id uint32) bool                        { _, ok := f.have[id]; return ok }
func (f *fakeLib) SetArtwork(uint32, int, string, []byte) error { return nil }
func (f *fakeLib) EnableDebugging() error                       { f.debug = true; return nil }

func appsServer(t *testing.T) (*Server, *fakeLib) {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "c.json"))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := apps.Open(filepath.Join(dir, "apps.json"))
	if err != nil {
		t.Fatal(err)
	}
	lib := &fakeLib{ready: true, next: 100, have: map[uint32]steamlib.Shortcut{}}
	return NewServer(store, &fakeDisp{}, "test", "tok", mustPair(t)).WithApps(cat, lib), lib
}

type appsResp struct {
	Apps  []appView       `json:"apps"`
	Steam steamlib.Status `json:"steam"`
}

func decodeApps(t *testing.T, body string) appsResp {
	t.Helper()
	var r appsResp
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return r
}

func TestAppsLifecycle(t *testing.T) {
	srv, lib := appsServer(t)
	h := srv.Handler()

	rec := do(h, "POST", "/api/apps", `{"name":"Notepad","exe":"/usr/bin/gedit","args":"-n","addToSteam":true}`)
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	r := decodeApps(t, rec.Body.String())
	if len(r.Apps) != 1 || !r.Apps[0].InSteam || r.Apps[0].SteamAppID != 101 {
		t.Fatalf("expected one app in Steam: %+v", r.Apps)
	}
	if got := lib.have[101]; got.Name != "Notepad" || got.Args != "-n" {
		t.Fatalf("steam got %+v", got)
	}
	id := r.Apps[0].ID

	if rec := do(h, "DELETE", "/api/apps/"+id+"/steam", ""); rec.Code != 200 || len(lib.have) != 0 {
		t.Fatalf("remove from steam: %d have=%d", rec.Code, len(lib.have))
	}
	if r := decodeApps(t, do(h, "GET", "/api/apps", "").Body.String()); r.Apps[0].InSteam || r.Apps[0].SteamAppID != 0 {
		t.Fatalf("should no longer be in Steam: %+v", r.Apps[0])
	}
	if rec := do(h, "POST", "/api/apps/"+id+"/steam", ""); rec.Code != 200 || len(lib.have) != 1 {
		t.Fatalf("re-add: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/apps/"+id, ""); rec.Code != 200 || len(lib.have) != 0 {
		t.Fatalf("delete: %d have=%d", rec.Code, len(lib.have))
	}
	if r := decodeApps(t, do(h, "GET", "/api/apps", "").Body.String()); len(r.Apps) != 0 {
		t.Fatalf("catalog should be empty: %+v", r.Apps)
	}
}

func TestAppsValidationAndSteamDown(t *testing.T) {
	srv, lib := appsServer(t)
	h := srv.Handler()
	for _, b := range []string{`{"name":"","exe":"/x"}`, `{"name":"a","exe":""}`, `{"name":"a\nb","exe":"/x"}`, `{"bogus":1}`} {
		if rec := do(h, "POST", "/api/apps", b); rec.Code != 400 {
			t.Errorf("%s: want 400 got %d", b, rec.Code)
		}
	}
	lib.ready = false
	rec := do(h, "POST", "/api/apps", `{"name":"X","exe":"/bin/x","addToSteam":true}`)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "saved") {
		t.Fatalf("want 502 with saved note, got %d %s", rec.Code, rec.Body)
	}
	r := decodeApps(t, do(h, "GET", "/api/apps", "").Body.String())
	if len(r.Apps) != 1 || r.Apps[0].InSteam || r.Steam.Ready {
		t.Fatalf("app should be saved but not in Steam: %+v", r)
	}
	if rec := do(h, "POST", "/api/steam/enable-debugging", ""); rec.Code != 200 || !lib.debug {
		t.Fatalf("enable: %d", rec.Code)
	}
}

func TestAppsRemoteAccess(t *testing.T) {
	srv, _ := appsServer(t)
	h := srv.Handler()
	const remote = "192.168.1.50:5555"
	if rec := doFrom(h, remote, "GET", "/api/apps", "", "Bearer tok"); rec.Code != 200 {
		t.Fatalf("remote read with token: %d", rec.Code)
	}
	if rec := doFrom(h, remote, "GET", "/api/apps", "", ""); rec.Code != 401 {
		t.Fatalf("remote read without token: %d", rec.Code)
	}
	for _, c := range [][2]string{{"POST", "/api/apps"}, {"DELETE", "/api/apps/x"}, {"POST", "/api/apps/x/steam"}, {"DELETE", "/api/apps/x/steam"}, {"POST", "/api/steam/enable-debugging"}} {
		if rec := doFrom(h, remote, c[0], c[1], `{"name":"a","exe":"/bin/sh"}`, "Bearer tok"); rec.Code != 403 {
			t.Errorf("%s %s from a remote host: want 403 got %d", c[0], c[1], rec.Code)
		}
	}
}
