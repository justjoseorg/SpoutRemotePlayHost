package server

import (
	"errors"
	"net/http"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/apps"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/artwork"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlib"
)

// WithApps enables the app catalog and its Steam library integration.
func (s *Server) WithApps(store *apps.Store, lib steamlib.Library) *Server {
	s.apps, s.lib = store, lib
	return s
}

type appView struct {
	apps.App
	InSteam bool `json:"inSteam"`
}

func (s *Server) appsEnabled(w http.ResponseWriter) bool {
	if s.apps == nil || s.lib == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("apps are not enabled"))
		return false
	}
	return true
}

func (s *Server) appList() map[string]any {
	st := s.lib.Status()
	list := s.apps.List()
	out := make([]appView, 0, len(list))
	for _, a := range list {
		v := appView{App: a}
		v.InSteam = st.Ready && a.SteamAppID != 0 && s.lib.Exists(a.SteamAppID)
		out = append(out, v)
	}
	return map[string]any{"apps": out, "steam": st, "artwork": s.art != nil && s.art.HasKey()}
}

// listApps is readable by paired devices so the Decky plugin can sync the catalog.
func (s *Server) listApps(w http.ResponseWriter, _ *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	writeJSON(w, 200, s.appList())
}

type appBody struct {
	Name       string `json:"name"`
	Exe        string `json:"exe"`
	Args       string `json:"args"`
	StartDir   string `json:"startDir"`
	AddToSteam bool   `json:"addToSteam"`
}

func (s *Server) addApp(w http.ResponseWriter, r *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	var b appBody
	if !decodeBody(w, r, &b) {
		return
	}
	a, err := s.apps.Add(apps.App{Name: b.Name, Exe: b.Exe, Args: b.Args, StartDir: b.StartDir})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if b.AddToSteam {
		if err := s.addToSteam(a); err != nil {
			writeErr(w, http.StatusBadGateway, errors.New("saved, but could not add to Steam: "+err.Error()))
			return
		}
	}
	writeJSON(w, 200, s.appList())
}

func (s *Server) addToSteam(a apps.App) error {
	if a.SteamAppID != 0 && s.lib.Exists(a.SteamAppID) {
		return nil
	}
	id, err := s.lib.Add(steamlib.Shortcut{Name: a.Name, Exe: a.Exe, StartDir: a.StartDir, Args: a.Args})
	if err != nil {
		return err
	}
	if err := s.apps.SetSteamAppID(a.ID, id); err != nil {
		return err
	}
	s.applyArtwork(a.Name, id)
	return nil
}

// applyArtwork is best effort: a shortcut without art is still usable.
func (s *Server) applyArtwork(name string, id uint32) error {
	if s.art == nil {
		return errors.New("artwork is not enabled")
	}
	imgs, err := s.art.Find(name)
	if err != nil {
		return err
	}
	for _, im := range imgs {
		if err := s.lib.SetArtwork(id, im.Kind, im.Ext, im.Data); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) steamArtwork(w http.ResponseWriter, r *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	a, err := s.apps.Get(r.PathValue("id"))
	if err != nil || a.SteamAppID == 0 || !s.lib.Exists(a.SteamAppID) {
		writeErr(w, http.StatusNotFound, errors.New("app is not in Steam"))
		return
	}
	if err := s.applyArtwork(a.Name, a.SteamAppID); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, 200, s.appList())
}

func (s *Server) setArtworkKey(w http.ResponseWriter, r *http.Request) {
	if s.art == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("artwork is not enabled"))
		return
	}
	var b struct {
		Key string `json:"key"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.art.SetKey(b.Key); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, s.appList())
}

// WithArtwork enables SteamGridDB artwork for shortcuts added to Steam.
func (s *Server) WithArtwork(c *artwork.Client) *Server {
	s.art = c
	return s
}

func (s *Server) removeFromSteam(a apps.App) error {
	if a.SteamAppID == 0 {
		return nil
	}
	if s.lib.Status().Ready {
		if err := s.lib.Remove(a.SteamAppID); err != nil {
			return err
		}
	} else if s.lib.Exists(a.SteamAppID) {
		return errors.New("steam is not controllable right now")
	}
	return s.apps.SetSteamAppID(a.ID, 0)
}

func (s *Server) steamAdd(w http.ResponseWriter, r *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	a, err := s.apps.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.addToSteam(a); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, 200, s.appList())
}

func (s *Server) steamRemove(w http.ResponseWriter, r *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	a, err := s.apps.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.removeFromSteam(a); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, 200, s.appList())
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	a, err := s.apps.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.removeFromSteam(a); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if err := s.apps.Remove(a.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, s.appList())
}

func (s *Server) enableSteam(w http.ResponseWriter, _ *http.Request) {
	if !s.appsEnabled(w) {
		return
	}
	if err := s.lib.EnableDebugging(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, s.appList())
}
