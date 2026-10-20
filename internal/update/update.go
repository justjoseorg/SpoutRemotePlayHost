// Package update checks GitHub for newer host releases and installs them.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const repo = "justjoseorg/SpoutRemotePlayHost"

// Status is what the web UI shows.
type Status struct {
	Current string `json:"current"`
	// Supported is false for dev builds and platforms without a release package.
	Supported  bool      `json:"supported"`
	Latest     string    `json:"latest,omitempty"`
	Available  bool      `json:"available"`
	URL        string    `json:"url,omitempty"`
	Checked    time.Time `json:"checked,omitempty"`
	Error      string    `json:"error,omitempty"`
	Auto       bool      `json:"auto"`
	Installing bool      `json:"installing"`
}

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

type release struct {
	Tag    string  `json:"tag_name"`
	URL    string  `json:"html_url"`
	Assets []asset `json:"assets"`
}

// state is persisted next to the host config.
type state struct {
	Auto     bool   `json:"auto"`
	Notified string `json:"notified,omitempty"`
}

// Updater checks for and installs updates. The exported fields are for tests.
type Updater struct {
	Current string
	API     string // GitHub API base
	// Downloads is the only URL prefix packages may be downloaded from.
	Downloads string
	Client    *http.Client
	Dir       string // where packages are downloaded
	// Apply installs a downloaded, verified package; the host stops or restarts.
	Apply func(path string) error
	OS    string

	statePath string
	mu        sync.Mutex
	st        state
	last      Status
	pkg       *asset
	busy      bool
}

// New returns an updater for this build; settings are kept in statePath.
func New(current, statePath string) *Updater {
	u := &Updater{
		Current:   current,
		API:       "https://api.github.com",
		Downloads: "https://github.com/" + repo + "/releases/download/",
		Client:    &http.Client{Timeout: 10 * time.Minute},
		Dir:       filepath.Join(os.TempDir(), "spout-update"),
		Apply:     apply,
		OS:        runtime.GOOS,
		statePath: statePath,
	}
	if b, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(b, &u.st)
	}
	return u
}

// Cleanup removes packages left by a previous update.
func (u *Updater) Cleanup() { _ = os.RemoveAll(u.Dir) }

var semverRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

func parse(v string) ([3]int, bool) {
	m := semverRE.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return [3]int{}, false
	}
	var r [3]int
	for i := range r {
		r[i], _ = strconv.Atoi(m[i+1])
	}
	return r, true
}

// newer reports whether version a is newer than b.
func newer(a, b string) bool {
	x, ok1 := parse(a)
	y, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

// packageName is the release asset this platform installs.
func (u *Updater) packageName(version string) string {
	switch {
	case u.OS == "windows":
		return "SpoutRemotePlayHost-Setup-v" + version + ".exe"
	case u.OS == "linux" && runtime.GOARCH == "amd64":
		return "spout-host-v" + version + "-linux-amd64.tar.gz"
	}
	return ""
}

func (u *Updater) supported() bool {
	_, ok := parse(u.Current)
	return ok && u.packageName("0.0.0") != ""
}

// Status returns the result of the last check.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.statusLocked()
}

func (u *Updater) statusLocked() Status {
	s := u.last
	s.Current, s.Supported, s.Auto, s.Installing = u.Current, u.supported(), u.st.Auto, u.busy
	return s
}

// Check asks GitHub for the latest release.
func (u *Updater) Check(ctx context.Context) Status {
	if !u.supported() {
		return u.Status()
	}
	rel, err := u.latest(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.last = Status{Checked: time.Now()}
	u.pkg = nil
	if err != nil {
		u.last.Error = err.Error()
		return u.statusLocked()
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	u.last.Latest, u.last.URL = latest, rel.URL
	if newer(latest, u.Current) {
		for i := range rel.Assets {
			if rel.Assets[i].Name == u.packageName(latest) {
				u.pkg = &rel.Assets[i]
			}
		}
		if u.pkg == nil {
			u.last.Error = "release v" + latest + " has no package for this system yet"
		} else {
			u.last.Available = true
		}
	}
	return u.statusLocked()
}

func (u *Updater) latest(ctx context.Context) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u.API+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checking for updates: GitHub returned %s", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	return &rel, nil
}

// SetAuto turns automatic installs on or off.
func (u *Updater) SetAuto(on bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.st.Auto = on
	return u.saveLocked()
}

func (u *Updater) saveLocked() error {
	b, _ := json.Marshal(u.st)
	return os.WriteFile(u.statePath, b, 0o600)
}

// ErrNoUpdate means there is nothing to install.
var ErrNoUpdate = errors.New("no update available")

// Install downloads and verifies the latest package, then applies it in the background
// (the host stops or restarts shortly after Install returns).
func (u *Updater) Install(ctx context.Context) error {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return errors.New("an update is already being installed")
	}
	pkg := u.pkg
	if !u.last.Available || pkg == nil {
		u.mu.Unlock()
		return ErrNoUpdate
	}
	u.busy = true
	u.mu.Unlock()
	path, err := u.download(ctx, *pkg)
	if err != nil {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
		return err
	}
	log.Printf("update: installing %s", pkg.Name)
	go func() {
		// Let the HTTP response reach the UI first.
		time.Sleep(500 * time.Millisecond)
		if err := u.Apply(path); err != nil {
			log.Println("update failed:", err)
			u.mu.Lock()
			u.busy = false
			u.last.Error = "installing the update failed: " + err.Error()
			u.mu.Unlock()
		}
	}()
	return nil
}

func (u *Updater) download(ctx context.Context, a asset) (string, error) {
	if !strings.HasPrefix(a.URL, u.Downloads) || a.Name != filepath.Base(a.Name) {
		return "", fmt.Errorf("refusing to download %s", a.URL)
	}
	want, ok := strings.CutPrefix(a.Digest, "sha256:")
	if !ok || len(want) != 64 {
		return "", errors.New("the release has no SHA-256 digest to verify the download")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading the update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading the update: %s", resp.Status)
	}
	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(u.Dir, a.Name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 512<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != strings.ToLower(want) {
		err = errors.New("the download doesn't match the release's SHA-256 digest")
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// Run checks after first and then every interval. It calls notify once per new version,
// or installs it when automatic updates are on and busy reports no stream.
func (u *Updater) Run(stop <-chan struct{}, first, interval time.Duration, notify func(Status), busy func() bool) {
	if !u.supported() {
		return
	}
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		s := u.Check(ctx)
		next := interval
		if s.Available {
			u.mu.Lock()
			fresh := u.st.Notified != s.Latest
			if fresh {
				u.st.Notified = s.Latest
				_ = u.saveLocked()
			}
			u.mu.Unlock()
			switch {
			case s.Auto && busy():
				// Retry soon; a stream is running.
				next = 10 * time.Minute
			case s.Auto:
				if err := u.Install(ctx); err != nil {
					log.Println("automatic update:", err)
				}
			case fresh && notify != nil:
				notify(s)
			}
		}
		cancel()
		t.Reset(next)
	}
}
