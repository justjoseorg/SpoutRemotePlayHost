package server

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/apps"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/picker"
)

// browseApp opens a file dialog on this PC's desktop: browsers can't hand a web page real paths.
func (s *Server) browseApp(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Start string `json:"start"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	p, err := s.pick(r.Context(), b.Start)
	if errors.Is(err, picker.ErrBusy) {
		writeErr(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, map[string]string{"path": p})
}

type shortcutView struct {
	AppID    uint32 `json:"appId"`
	Name     string `json:"name"`
	Exe      string `json:"exe"`
	Args     string `json:"args,omitempty"`
	StartDir string `json:"startDir,omitempty"`
	Imported bool   `json:"imported"`
}

func (s *Server) shortcutViews() ([]shortcutView, error) {
	list, err := s.shortcuts()
	if err != nil {
		return nil, err
	}
	have := map[uint32]bool{}
	for _, a := range s.apps.List() {
		if a.SteamAppID != 0 {
			have[a.SteamAppID] = true
		}
	}
	out := make([]shortcutView, 0, len(list))
	for _, e := range list {
		out = append(out, shortcutView{AppID: e.AppID, Name: e.Name, Exe: e.Exe, Args: e.Args, StartDir: e.StartDir, Imported: have[e.AppID]})
	}
	return out, nil
}

// steamShortcuts lists the non-Steam games already in this PC's Steam library.
func (s *Server) steamShortcuts(w http.ResponseWriter, _ *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	list, err := s.shortcutViews()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, map[string]any{"shortcuts": list})
}

// importApps adds existing Steam shortcuts to the catalog, linked to their Steam entries.
func (s *Server) importApps(w http.ResponseWriter, r *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	var b struct {
		AppIDs []uint32 `json:"appIds"`
		All    bool     `json:"all"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	list, err := s.shortcutViews()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	want := map[uint32]bool{}
	for _, id := range b.AppIDs {
		want[id] = true
	}
	imported, skipped := 0, []string{}
	for _, e := range list {
		if !b.All && !want[e.AppID] {
			continue
		}
		if e.Imported {
			continue
		}
		a, err := s.apps.Add(apps.App{Name: truncate(e.Name, 80), Exe: e.Exe, Args: e.Args, StartDir: e.StartDir})
		if err == nil {
			err = s.apps.SetSteamAppID(a.ID, e.AppID)
		}
		if err != nil {
			skipped = append(skipped, e.Name+": "+err.Error())
			continue
		}
		imported++
	}
	out := s.appList()
	out["imported"], out["skipped"] = imported, skipped
	writeJSON(w, 200, out)
}

// truncate shortens s to at most n bytes without splitting a character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
