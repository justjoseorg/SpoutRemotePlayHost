package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/artwork"
)

func TestChooseArtwork(t *testing.T) {
	var cdn *httptest.Server
	cdn = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/"):
			w.Write([]byte(`{"data":[{"id":7,"name":"Game"}]}`))
		case strings.HasPrefix(r.URL.Path, "/logos/game/7"):
			w.Write([]byte(`{"data":[{"url":"` + cdn.URL + `/l.png","mime":"image/png"}]}`))
		case r.URL.Path == "/l.png":
			w.Write([]byte("PNG"))
		default:
			w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer cdn.Close()
	srv, lib := appsServer(t)
	art := artwork.Open(filepath.Join(t.TempDir(), "key"))
	art.Base = cdn.URL
	_ = art.SetKey("k")
	srv.WithArtwork(art)
	h := srv.Handler()

	r := decodeApps(t, do(h, "POST", "/api/apps", `{"name":"Game","exe":"/bin/g","addToSteam":true}`).Body.String())
	id := r.Apps[0].ID
	rec := do(h, "GET", "/api/apps/"+id+"/artwork/options", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/l.png") {
		t.Fatalf("options: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "PUT", "/api/apps/"+id+"/artwork", `{"kind":2,"url":"`+cdn.URL+`/l.png"}`); rec.Code != 200 || lib.art[2] != "png" {
		t.Fatalf("set: %d %s %v", rec.Code, rec.Body, lib.art)
	}
	if rec := do(h, "PUT", "/api/apps/"+id+"/artwork", `{"kind":2,"url":"https://evil.example/x.png"}`); rec.Code != 400 {
		t.Fatalf("foreign url: want 400 got %d", rec.Code)
	}
	do(h, "DELETE", "/api/apps/"+id+"/steam", "")
	if rec := do(h, "GET", "/api/apps/"+id+"/artwork/options", ""); rec.Code != 404 {
		t.Fatalf("not in Steam: want 404 got %d", rec.Code)
	}
	for _, c := range [][2]string{{"GET", "/api/apps/" + id + "/artwork/options"}, {"PUT", "/api/apps/" + id + "/artwork"}} {
		if rec := doFrom(h, "192.168.1.50:5555", c[0], c[1], `{}`, "Bearer "+"tok"); rec.Code != 403 {
			t.Errorf("remote %s: want 403 got %d", c[1], rec.Code)
		}
	}
}
