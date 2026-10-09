// Package steamlib adds and removes non-Steam shortcuts in the running Steam client through its
// local CEF debugging endpoint, so changes appear (and are streamable) without restarting Steam.
package steamlib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/coder/websocket"
)

// Shortcut describes a non-Steam game entry.
type Shortcut struct {
	Name     string
	Exe      string
	StartDir string
	Args     string
}

// Status reports whether the Steam client can currently be controlled.
type Status struct {
	Ready        bool   `json:"ready"`
	DebugEnabled bool   `json:"debugEnabled"`
	Hint         string `json:"hint,omitempty"`
}

// Library manages shortcuts in the Steam client.
type Library interface {
	Status() Status
	Add(Shortcut) (uint32, error)
	Remove(id uint32) error
	Exists(id uint32) bool
	EnableDebugging() error
}

const debugFlag = ".cef-enable-remote-debugging"

// CEF drives Steam through http://<addr>/json (Steam's CEF remote debugging port).
type CEF struct{ Addr string }

func NewCEF() *CEF { return &CEF{Addr: "127.0.0.1:8080"} }

// Roots lists likely Steam installation directories.
func Roots() []string {
	if runtime.GOOS == "windows" {
		var out []string
		for _, e := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if p := os.Getenv(e); p != "" {
				out = append(out, filepath.Join(p, "Steam"))
			}
		}
		return out
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(h, ".local/share/Steam"),
		filepath.Join(h, ".steam/steam"),
		filepath.Join(h, ".var/app/com.valvesoftware.Steam/.local/share/Steam"),
	}
}

func debugFlagPresent() bool {
	for _, r := range Roots() {
		if _, err := os.Stat(filepath.Join(r, debugFlag)); err == nil {
			return true
		}
	}
	return false
}

// EnableDebugging creates the flag file Steam checks at startup; Steam must be restarted afterwards.
func (c *CEF) EnableDebugging() error {
	done := false
	for _, r := range Roots() {
		if st, err := os.Stat(r); err != nil || !st.IsDir() {
			continue
		}
		if err := os.WriteFile(filepath.Join(r, debugFlag), nil, 0o644); err != nil {
			return err
		}
		done = true
	}
	if !done {
		return errors.New("steam installation not found")
	}
	return nil
}

func (c *CEF) target(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+c.Addr+"/json", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tabs []struct {
		Title string `json:"title"`
		WS    string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return "", err
	}
	for _, t := range tabs {
		if t.Title == "SharedJSContext" && t.WS != "" {
			return t.WS, nil
		}
	}
	return "", errors.New("steam's UI context is not available yet")
}

// eval runs a JavaScript expression in Steam's UI context and returns its JSON-encoded result.
func (c *CEF) eval(expr string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	url, err := c.target(ctx)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	msg, _ := json.Marshal(map[string]any{"id": 1, "method": "Runtime.evaluate", "params": map[string]any{
		"expression": expr, "awaitPromise": true, "returnByValue": true}})
	if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
		return nil, err
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		var r struct {
			ID     int `json:"id"`
			Result struct {
				Result struct {
					Value json.RawMessage `json:"value"`
				} `json:"result"`
				Exception *struct {
					Text string `json:"text"`
				} `json:"exceptionDetails"`
			} `json:"result"`
		}
		if json.Unmarshal(data, &r) != nil || r.ID != 1 {
			continue
		}
		if r.Result.Exception != nil {
			return nil, fmt.Errorf("steam: %s", r.Result.Exception.Text)
		}
		return r.Result.Result.Value, nil
	}
}

func (c *CEF) Status() Status {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.target(ctx); err == nil {
		return Status{Ready: true, DebugEnabled: true}
	}
	st := Status{DebugEnabled: debugFlagPresent()}
	if st.DebugEnabled {
		st.Hint = "Restart Steam so it picks up remote debugging."
	} else {
		st.Hint = "Enable Steam integration, then restart Steam once."
	}
	return st
}

func js(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (c *CEF) Add(s Shortcut) (uint32, error) {
	dir := s.StartDir
	if dir == "" {
		dir = filepath.Dir(s.Exe)
	}
	// Steam registers the new entry asynchronously, so wait for it before renaming.
	expr := fmt.Sprintf(`(async()=>{const id=await SteamClient.Apps.AddShortcut(%s,%s,%s,%s);`+
		`for(let i=0;i<50&&!appStore.GetAppOverviewByAppID(id);i++)await new Promise(r=>setTimeout(r,100));`+
		`SteamClient.Apps.SetShortcutName(id,%s);SteamClient.Apps.SetShortcutLaunchOptions(id,%s);return id})()`,
		js(s.Name), js(s.Exe), js(dir), js(s.Args), js(s.Name), js(s.Args))
	raw, err := c.eval(expr)
	if err != nil {
		return 0, err
	}
	var id uint32
	if err := json.Unmarshal(raw, &id); err != nil || id == 0 {
		return 0, fmt.Errorf("steam returned no app id (%s)", raw)
	}
	return id, nil
}

func (c *CEF) Remove(id uint32) error {
	_, err := c.eval(fmt.Sprintf("SteamClient.Apps.RemoveShortcut(%d)", id))
	return err
}

func (c *CEF) Exists(id uint32) bool {
	raw, err := c.eval(fmt.Sprintf("appStore.GetAppOverviewByAppID(%d)!=null", id))
	var ok bool
	return err == nil && json.Unmarshal(raw, &ok) == nil && ok
}
