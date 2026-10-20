package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/update"
)

// WithUpdater enables the update endpoints.
func (s *Server) WithUpdater(u *update.Updater) *Server {
	s.upd = u
	return s
}

func (s *Server) updater(w http.ResponseWriter) bool {
	if s.upd == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("updates are not enabled"))
		return false
	}
	return true
}

// updateStatus returns the last check; ?check=1 asks GitHub again.
func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	if !s.updater(w) {
		return
	}
	if r.URL.Query().Get("check") == "1" {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		writeJSON(w, 200, s.upd.Check(ctx))
		return
	}
	writeJSON(w, 200, s.upd.Status())
}

// installUpdate downloads and starts installing the latest release; the host restarts.
func (s *Server) installUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.updater(w) {
		return
	}
	if err := s.upd.Install(r.Context()); err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, update.ErrNoUpdate) {
			code = http.StatusConflict
		}
		writeErr(w, code, err)
		return
	}
	writeJSON(w, 200, s.upd.Status())
}

func (s *Server) setAutoUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.updater(w) {
		return
	}
	var b struct {
		Auto bool `json:"auto"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.upd.SetAuto(b.Auto); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, 200, s.upd.Status())
}
