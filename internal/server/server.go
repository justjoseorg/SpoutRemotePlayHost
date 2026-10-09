package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/justjoseorg/SpigotRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpigotRemotePlayHost/internal/display"
)

//go:embed web
var webFS embed.FS

type Server struct {
	cfg     *config.Store
	disp    display.Manager
	version string
}

// New returns the HTTP handler. Requests from non-loopback addresses must send "Authorization: Bearer <token>".
func New(cfg *config.Store, disp display.Manager, version, token string) http.Handler {
	s := &Server{cfg: cfg, disp: disp, version: version}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("POST /api/monitor/create", s.create)
	mux.HandleFunc("POST /api/monitor/destroy", s.destroy)
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	return requireToken(mux, token)
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
	if err := s.cfg.Set(m); err != nil {
		writeErr(w, 400, err)
		return
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

func requireToken(next http.Handler, token string) http.Handler {
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
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeErr(w, http.StatusUnauthorized, errors.New("missing or invalid token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
