package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/update"
)

func TestUpdateEndpoints(t *testing.T) {
	sum := sha256.Sum256([]byte("setup"))
	var gh *httptest.Server
	gh = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.9.0", "assets": []map[string]string{{
				"name": "SpoutRemotePlayHost-Setup-v0.9.0.exe", "browser_download_url": gh.URL + "/dl/s.exe",
				"digest": "sha256:" + hex.EncodeToString(sum[:])}}})
			return
		}
		_, _ = w.Write([]byte("setup"))
	}))
	defer gh.Close()
	dir := t.TempDir()
	applied := make(chan string, 1)
	u := update.New("0.8.0", filepath.Join(dir, "update.json"))
	u.API, u.Downloads, u.Client, u.Dir, u.OS = gh.URL, gh.URL+"/dl/", gh.Client(), dir, "windows"
	u.Apply = func(p string) error { applied <- p; return nil }

	srv, _ := appsServer(t)
	h := srv.WithUpdater(u).Handler()
	if rec := do(h, "POST", "/api/update/install", ""); rec.Code != 409 {
		t.Fatalf("install before check: %d %s", rec.Code, rec.Body)
	}
	var st update.Status
	rec := do(h, "GET", "/api/update?check=1", "")
	if json.Unmarshal(rec.Body.Bytes(), &st) != nil || !st.Available || st.Latest != "0.9.0" {
		t.Fatalf("check: %d %s", rec.Code, rec.Body)
	}
	if rec := doFrom(h, "192.168.1.50:5555", "POST", "/api/update/install", "", "Bearer "+"tok"); rec.Code != 403 {
		t.Fatalf("remote install: want 403 got %d", rec.Code)
	}
	if rec := do(h, "PUT", "/api/update/auto", `{"auto":true}`); rec.Code != 200 || !u.Status().Auto {
		t.Fatalf("auto: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/api/update/install", ""); rec.Code != 200 {
		t.Fatalf("install: %d %s", rec.Code, rec.Body)
	}
	select {
	case <-applied:
	case <-time.After(3 * time.Second):
		t.Fatal("not applied")
	}
}
