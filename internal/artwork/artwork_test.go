package artwork

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestFind(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/img.png" && r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/autocomplete/"):
			w.Write([]byte(`{"data":[{"id":7}]}`))
		case strings.HasPrefix(r.URL.Path, "/grids/game/7"):
			w.Write([]byte(`{"data":[{"url":"` + srv.URL + `/img.png","mime":"image/png"}]}`))
		case r.URL.Path == "/img.png":
			w.Write([]byte("PNG"))
		default:
			w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()
	c := Open(filepath.Join(t.TempDir(), "key"))
	c.Base = srv.URL
	if _, err := c.Find("x"); err == nil {
		t.Fatal("expected missing key error")
	}
	if err := c.SetKey("k"); err != nil {
		t.Fatal(err)
	}
	imgs, err := c.Find("My Game")
	if err != nil || len(imgs) != 2 || imgs[0].Kind != Portrait || string(imgs[0].Data) != "PNG" {
		t.Fatalf("%v %+v", err, imgs)
	}
	if !Open(c.path).HasKey() {
		t.Fatal("key not persisted")
	}
}
