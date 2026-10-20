package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/picker"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlib"
)

func TestBrowseApp(t *testing.T) {
	srv, _ := appsServer(t)
	var gotStart string
	srv.pick = func(_ context.Context, start string) (string, error) {
		gotStart = start
		return `C:\Games\Celeste\Celeste.exe`, nil
	}
	h := srv.Handler()
	rec := do(h, "POST", "/api/apps/browse", `{"start":"C:\\Games"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `Celeste.exe`) || gotStart != `C:\Games` {
		t.Fatalf("browse: %d %s start=%q", rec.Code, rec.Body, gotStart)
	}
	srv.pick = func(context.Context, string) (string, error) { return "", picker.ErrBusy }
	if rec := do(h, "POST", "/api/apps/browse", `{}`); rec.Code != 409 {
		t.Fatalf("busy: want 409 got %d", rec.Code)
	}
	if rec := doFrom(h, "192.168.1.50:5555", "POST", "/api/apps/browse", `{}`, "Bearer "+"tok"); rec.Code != 403 {
		t.Fatalf("remote browse: want 403 got %d", rec.Code)
	}
}

func TestImportSteamShortcuts(t *testing.T) {
	srv, lib := appsServer(t)
	lib.have[3000000001] = steamlib.Shortcut{Name: "Celeste"}
	srv.shortcuts = func() ([]steamlib.Existing, error) {
		return []steamlib.Existing{
			{AppID: 3000000001, Name: "Celeste", Exe: `C:\Games\Celeste.exe`, StartDir: `C:\Games`},
			{AppID: 3000000002, Name: strings.Repeat("x", 90), Exe: `C:\x.exe`},
			{AppID: 3000000003, Name: "Bad", Exe: "C:\\bad\n.exe"},
		}, nil
	}
	h := srv.Handler()

	var list struct {
		Shortcuts []shortcutView `json:"shortcuts"`
	}
	if err := json.Unmarshal(do(h, "GET", "/api/steam/shortcuts", "").Body.Bytes(), &list); err != nil || len(list.Shortcuts) != 3 || list.Shortcuts[0].Imported {
		t.Fatalf("list: %v %+v", err, list)
	}

	rec := do(h, "POST", "/api/apps/import", `{"appIds":[3000000001]}`)
	r := decodeApps(t, rec.Body.String())
	if rec.Code != 200 || len(r.Apps) != 1 || r.Apps[0].SteamAppID != 3000000001 || !r.Apps[0].InSteam || r.Apps[0].StartDir != `C:\Games` {
		t.Fatalf("import one: %d %+v", rec.Code, r.Apps)
	}

	rec = do(h, "POST", "/api/apps/import", `{"all":true}`)
	var res struct {
		Apps     []appView `json:"apps"`
		Imported int       `json:"imported"`
		Skipped  []string  `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || len(res.Apps) != 2 || len(res.Skipped) != 1 || !strings.HasPrefix(res.Skipped[0], "Bad") {
		t.Fatalf("import all (already-imported one must not duplicate): %+v", res)
	}
	if n := len(res.Apps[1].Name); n != 80 {
		t.Fatalf("long name should be cut to 80 bytes, got %d", n)
	}
	if err := json.Unmarshal(do(h, "GET", "/api/steam/shortcuts", "").Body.Bytes(), &list); err != nil || !list.Shortcuts[0].Imported || !list.Shortcuts[1].Imported {
		t.Fatalf("imported flags: %+v", list)
	}
	for _, p := range []string{"/api/apps/import", "/api/steam/shortcuts"} {
		m := "POST"
		if strings.HasSuffix(p, "shortcuts") {
			m = "GET"
		}
		if rec := doFrom(h, "192.168.1.50:5555", m, p, `{"all":true}`, "Bearer "+"tok"); rec.Code != 403 {
			t.Errorf("remote %s: want 403 got %d", p, rec.Code)
		}
	}
}

func TestTruncateKeepsWholeCharacters(t *testing.T) {
	if got := truncate("ab\u00e9", 3); got != "ab" {
		t.Fatalf("got %q", got)
	}
}
