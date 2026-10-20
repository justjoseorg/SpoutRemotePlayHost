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

func TestCleanKey(t *testing.T) {
	for in, want := range map[string]string{"abc123": "abc123", " Key: abc123\n": "abc123", "API key abc": "abc", "  ": ""} {
		if got := cleanKey(in); got != want {
			t.Errorf("cleanKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOptionsAndFetch(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/autocomplete/"):
			w.Write([]byte(`{"data":[{"id":7,"name":"Seven"},{"id":8,"name":"Eight"}]}`))
		case strings.HasPrefix(r.URL.Path, "/heroes/game/8"):
			w.Write([]byte(`{"data":[{"url":"` + srv.URL + `/a.png","thumb":"` + srv.URL + `/a_t.png","mime":"image/png"},` +
				`{"url":"https://evil.example/x.png","mime":"image/png"},{"url":"` + srv.URL + `/b.gif","mime":"image/gif"}]}`))
		case r.URL.Path == "/a.png":
			w.Write([]byte("PNG"))
		default:
			w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()
	c := Open(filepath.Join(t.TempDir(), "key"))
	c.Base = srv.URL
	_ = c.SetKey("Key: k")
	if c.key != "k" {
		t.Fatalf("key = %q", c.key)
	}
	ch, err := c.Options("x", 8)
	if err != nil || ch.Game != 8 || len(ch.Games) != 2 || len(ch.Kinds[Hero]) != 1 || ch.Kinds[Hero][0].Thumb != srv.URL+"/a_t.png" {
		t.Fatalf("%v %+v", err, ch)
	}
	img, err := c.Fetch(Hero, srv.URL+"/a.png")
	if err != nil || img.Ext != "png" || string(img.Data) != "PNG" || img.Kind != Hero {
		t.Fatalf("fetch: %v %+v", err, img)
	}
	for _, u := range []string{"https://evil.example/x.png", "file:///etc/passwd", srv.URL + "/a.gif"} {
		if _, err := c.Fetch(Hero, u); err == nil {
			t.Errorf("%s: want error", u)
		}
	}
	if _, err := c.Fetch(9, srv.URL+"/a.png"); err == nil {
		t.Error("bad kind: want error")
	}
	if !c.allowed("https://cdn2.steamgriddb.com/grid/x.png") || c.allowed("http://cdn2.steamgriddb.com/x.png") || c.allowed("https://steamgriddb.com.evil.io/x.png") {
		t.Error("allowed() host rules")
	}
}
