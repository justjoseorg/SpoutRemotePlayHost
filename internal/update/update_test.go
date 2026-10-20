package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.7.9", "0.7.8", true}, {"0.8.0", "0.7.10", true}, {"v1.0.0", "0.9.9", true},
		{"0.7.8", "0.7.8", false}, {"0.7.7", "0.7.8", false}, {"0.7.9", "dev", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

type fakeGitHub struct {
	*httptest.Server
	tag, digest string
	body        []byte
}

func newFake(t *testing.T, tag string, body []byte) *fakeGitHub {
	f := &fakeGitHub{tag: tag, body: body}
	sum := sha256.Sum256(body)
	f.digest = "sha256:" + hex.EncodeToString(sum[:])
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + repo + "/releases/latest":
			v := f.tag[1:]
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": f.tag, "html_url": "https://example/rel",
				"assets": []map[string]string{{"name": "SpoutRemotePlayHost-Setup-v" + v + ".exe",
					"browser_download_url": f.URL + "/dl/setup.exe", "digest": f.digest}}})
		case "/dl/setup.exe":
			_, _ = w.Write(f.body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func testUpdater(t *testing.T, f *fakeGitHub, current string) (*Updater, chan string) {
	dir := t.TempDir()
	applied := make(chan string, 1)
	u := New(current, filepath.Join(dir, "update.json"))
	u.API, u.Downloads, u.Client, u.Dir, u.OS = f.URL, f.URL+"/dl/", f.Client(), filepath.Join(dir, "dl"), "windows"
	u.Apply = func(p string) error { applied <- p; return nil }
	return u, applied
}

func TestCheckAndInstall(t *testing.T) {
	f := newFake(t, "v0.7.9", []byte("setup"))
	u, applied := testUpdater(t, f, "0.7.8")
	s := u.Check(context.Background())
	if !s.Available || s.Latest != "0.7.9" || s.Error != "" {
		t.Fatalf("status %+v", s)
	}
	if err := u.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-applied:
		if b, _ := os.ReadFile(p); string(b) != "setup" {
			t.Fatalf("package %q", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not applied")
	}
	if !u.Status().Installing {
		t.Fatal("not marked installing")
	}
}

func TestUpToDateAndDev(t *testing.T) {
	f := newFake(t, "v0.7.8", []byte("x"))
	u, _ := testUpdater(t, f, "0.7.8")
	if s := u.Check(context.Background()); s.Available || s.Latest != "0.7.8" {
		t.Fatalf("status %+v", s)
	}
	if err := u.Install(context.Background()); err != ErrNoUpdate {
		t.Fatalf("got %v", err)
	}
	d, _ := testUpdater(t, f, "dev")
	if s := d.Check(context.Background()); s.Supported || !s.Checked.IsZero() {
		t.Fatalf("dev build checked: %+v", s)
	}
}

func TestRejectsBadDigestAndURL(t *testing.T) {
	f := newFake(t, "v0.7.9", []byte("setup"))
	f.digest = "sha256:" + fmt.Sprintf("%064d", 0)
	u, applied := testUpdater(t, f, "0.7.8")
	u.Check(context.Background())
	if err := u.Install(context.Background()); err == nil {
		t.Fatal("bad digest accepted")
	}
	f.digest = ""
	u.Check(context.Background())
	if err := u.Install(context.Background()); err == nil {
		t.Fatal("missing digest accepted")
	}
	u.Downloads = "https://github.com/"
	sum := sha256.Sum256([]byte("setup"))
	f.digest = "sha256:" + hex.EncodeToString(sum[:])
	u.Check(context.Background())
	if err := u.Install(context.Background()); err == nil {
		t.Fatal("foreign URL accepted")
	}
	if len(applied) != 0 || u.Status().Installing {
		t.Fatal("applied after a failure")
	}
}

func TestRunNotifiesOnceAndAutoInstalls(t *testing.T) {
	f := newFake(t, "v0.7.9", []byte("setup"))
	u, applied := testUpdater(t, f, "0.7.8")
	notified := make(chan Status, 4)
	stop := make(chan struct{})
	go u.Run(stop, time.Millisecond, 20*time.Millisecond, func(s Status) { notified <- s }, func() bool { return false })
	time.Sleep(200 * time.Millisecond)
	close(stop)
	if len(notified) != 1 {
		t.Fatalf("notified %d times", len(notified))
	}
	if len(applied) != 0 {
		t.Fatal("installed without auto")
	}
	// The notification is remembered across restarts.
	u2 := New("0.7.8", u.statePath)
	if u2.st.Notified != "0.7.9" {
		t.Fatalf("state %+v", u2.st)
	}

	_ = u.SetAuto(true)
	var busy atomic.Bool
	busy.Store(true)
	stop = make(chan struct{})
	go u.Run(stop, time.Millisecond, time.Hour, nil, busy.Load)
	time.Sleep(100 * time.Millisecond)
	close(stop)
	if len(applied) != 0 {
		t.Fatal("installed during a stream")
	}
	busy.Store(false)
	stop = make(chan struct{})
	defer close(stop)
	go u.Run(stop, time.Millisecond, time.Hour, nil, busy.Load)
	select {
	case <-applied:
	case <-time.After(3 * time.Second):
		t.Fatal("auto install didn't run")
	}
}
