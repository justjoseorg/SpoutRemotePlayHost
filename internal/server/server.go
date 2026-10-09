package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
)

//go:embed web
var webFS embed.FS

type Server struct {
	cfg     *config.Store
	disp    display.Manager
	version string
	pair    *pairing.Manager
}

// New returns the HTTP handler. Requests from non-loopback addresses must send "Authorization: Bearer <token>".
func New(cfg *config.Store, disp display.Manager, version, token string, pair *pairing.Manager) http.Handler {
	s := &Server{cfg: cfg, disp: disp, version: version, pair: pair}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("POST /api/monitor/create", s.create)
	mux.HandleFunc("POST /api/monitor/destroy", s.destroy)
	mux.HandleFunc("POST /api/pair/request", s.pairRequest)
	mux.HandleFunc("POST /api/pair/poll", s.pairPoll)
	mux.HandleFunc("GET /api/pair/pending", localOnly(s.pairPending))
	mux.HandleFunc("POST /api/pair/confirm", localOnly(s.pairConfirm))
	mux.HandleFunc("DELETE /api/pair/pending/{id}", localOnly(s.pairDeny))
	mux.HandleFunc("GET /api/clients", localOnly(s.clients))
	mux.HandleFunc("DELETE /api/clients/{id}", localOnly(s.revoke))
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	return requireToken(mux, token, pair)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"version": s.version, "monitorActive": s.disp.Active()})
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.cfg.Get())
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var m config.Monitor
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		writeErr(w, 400, err)
		return
	}
	old := s.cfg.Get()
	if err := s.cfg.Set(m); err != nil {
		writeErr(w, 400, err)
		return
	}
	// Re-create a live virtual display so resolution/refresh changes apply immediately.
	if s.disp.Active() && (old.Width != m.Width || old.Height != m.Height || old.RefreshHz != m.RefreshHz) {
		if err := s.disp.Create(display.Mode{Width: m.Width, Height: m.Height, RefreshHz: m.RefreshHz}); err != nil {
			writeErr(w, 500, fmt.Errorf("saved, but could not apply to the active monitor: %w", err))
			return
		}
	}
	writeJSON(w, 200, m)
}

func (s *Server) create(w http.ResponseWriter, _ *http.Request) {
	c := s.cfg.Get()
	err := s.disp.Create(display.Mode{Width: c.Width, Height: c.Height, RefreshHz: c.RefreshHz})
	if errors.Is(err, display.ErrNotImplemented) {
		writeErr(w, 501, err)
		return
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"monitorActive": true})
}

func (s *Server) destroy(w http.ResponseWriter, _ *http.Request) {
	if err := s.disp.Destroy(); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"monitorActive": false})
}

func requireToken(next http.Handler, token string, pair *pairing.Manager) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if ip := net.ParseIP(host); err == nil && ip != nil && ip.IsLoopback() {
			// Loopback is trusted, but block cross-site browser requests to it.
			if o := r.Header.Get("Origin"); o != "" && r.Method != http.MethodGet && !strings.HasSuffix(o, "//"+r.Host) {
				writeErr(w, http.StatusForbidden, errors.New("cross-origin request rejected"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		// Pairing endpoints are reachable without a token; they are gated by the PIN confirmed on this PC.
		if r.Method == http.MethodPost && (r.URL.Path == "/api/pair/request" || r.URL.Path == "/api/pair/poll") {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(auth), want) != 1 && !(pair != nil && strings.HasPrefix(auth, "Bearer ") && pair.Valid(auth[7:])) {
			writeErr(w, http.StatusUnauthorized, errors.New("missing or invalid token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
