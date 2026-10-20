package server

import (
	"errors"
	"net/http"
	"strconv"
)

// steamApp returns the catalog app {id} if it's in Steam.
func (s *Server) steamApp(w http.ResponseWriter, r *http.Request) (uint32, string, bool) {
	if !s.appsEnabled(w) {
		return 0, "", false
	}
	if s.art == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("artwork is not enabled"))
		return 0, "", false
	}
	a, err := s.apps.Get(r.PathValue("id"))
	if err != nil || a.SteamAppID == 0 || !s.lib.Exists(a.SteamAppID) {
		writeErr(w, http.StatusNotFound, errors.New("app is not in Steam"))
		return 0, "", false
	}
	return a.SteamAppID, a.Name, true
}

// artworkOptions lists the SteamGridDB images an app can use; ?game= picks another search match.
func (s *Server) artworkOptions(w http.ResponseWriter, r *http.Request) {
	_, name, ok := s.steamApp(w, r)
	if !ok {
		return
	}
	game, _ := strconv.Atoi(r.URL.Query().Get("game"))
	ch, err := s.art.Options(name, game)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, 200, ch)
}

// setArtwork applies one chosen image.
func (s *Server) setArtwork(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.steamApp(w, r)
	if !ok {
		return
	}
	var b struct {
		Kind int    `json:"kind"`
		URL  string `json:"url"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	img, err := s.art.Fetch(b.Kind, b.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.lib.SetArtwork(id, img.Kind, img.Ext, img.Data); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
