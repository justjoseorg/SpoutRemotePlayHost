package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/apps"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/artwork"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/power"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/session"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlib"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/wol"
)

//go:embed web
var webFS embed.FS

type Server struct {
	cfg     *config.Store
	disp    display.Manager
	version string
	pair    *pairing.Manager
	sess    *session.Controller
	apps    *apps.Store
	art     *artwork.Client
	lib     steamlib.Library
	handler http.Handler
	// shutdown powers the PC off; tests replace it.
	shutdown func() error
}

// Sessions returns the controller that ties Remote Play sessions to the virtual monitor.
func (s *Server) Sessions() *session.Controller { return s.sess }

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// New returns the HTTP handler. Non-loopback requests must send a bearer token.
func New(cfg *config.Store, disp display.Manager, version, token string, pair *pairing.Manager) http.Handler {
	return NewServer(cfg, disp, version, token, pair).Handler()
}

// NewServer is New but also exposes the session controller.
func NewServer(cfg *config.Store, disp display.Manager, version, token string, pair *pairing.Manager) *Server {
	s := &Server{cfg: cfg, disp: disp, version: version, pair: pair, sess: session.New(cfg, disp), shutdown: power.Shutdown}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("POST /api/monitor/create", s.create)
	mux.HandleFunc("POST /api/monitor/destroy", s.destroy)
	mux.HandleFunc("GET /api/displays", localOnly(s.displays))
	mux.HandleFunc("PUT /api/displays/keep", localOnly(s.putKeep))
	mux.HandleFunc("POST /api/session", s.session)
	mux.HandleFunc("POST /api/wake", s.wake)
	mux.HandleFunc("POST /api/power", s.power)
	mux.HandleFunc("GET /api/discover", s.discover)
	mux.HandleFunc("POST /api/pair/request", s.pairRequest)
	mux.HandleFunc("POST /api/pair/poll", s.pairPoll)
	mux.HandleFunc("GET /api/pair/pending", localOnly(s.pairPending))
	mux.HandleFunc("POST /api/pair/confirm", localOnly(s.pairConfirm))
	mux.HandleFunc("DELETE /api/pair/pending/{id}", localOnly(s.pairDeny))
	mux.HandleFunc("GET /api/clients", localOnly(s.clients))
	mux.HandleFunc("GET /api/steam/devices", localOnly(s.listSteamDevices))
	mux.HandleFunc("DELETE /api/clients/{id}", localOnly(s.revoke))
	mux.HandleFunc("GET /api/clients/{id}/config", localOnly(s.clientConfig))
	mux.HandleFunc("PUT /api/clients/{id}/config", localOnly(s.putClientConfig))
	mux.HandleFunc("DELETE /api/clients/{id}/config", localOnly(s.resetClientConfig))
	mux.HandleFunc("GET /api/apps", s.listApps)
	// Adding or removing apps runs programs on this PC, so only the local UI may do it.
	mux.HandleFunc("POST /api/apps", localOnly(s.addApp))
	mux.HandleFunc("DELETE /api/apps/{id}", localOnly(s.deleteApp))
	mux.HandleFunc("POST /api/apps/{id}/steam", localOnly(s.steamAdd))
	mux.HandleFunc("DELETE /api/apps/{id}/steam", localOnly(s.steamRemove))
	mux.HandleFunc("POST /api/apps/{id}/artwork", localOnly(s.steamArtwork))
	mux.HandleFunc("PUT /api/steam/artwork-key", localOnly(s.setArtworkKey))
	mux.HandleFunc("POST /api/steam/enable-debugging", localOnly(s.enableSteam))
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	s.handler = requireToken(mux, token, pair)
	return s
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

// deviceID identifies the paired device making the request; "" for the local UI or the master token.
func (s *Server) deviceID(r *http.Request) string {
	if s.pair == nil {
		return ""
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return s.pair.ClientID(auth[7:])
	}
	return ""
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.cfg.GetFor(s.deviceID(r)))
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var m config.Monitor
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := s.deviceID(r)
	old := s.cfg.GetFor(id)
	if err := s.cfg.SetFor(id, m); err != nil {
		writeErr(w, 400, err)
		return
	}
	// Re-create a live virtual display so resolution/refresh changes apply immediately.
	if s.sess.OwnedBy(id) && (old.Width != m.Width || old.Height != m.Height || old.RefreshHz != m.RefreshHz) {
		if err := s.sess.Reapply(id); err != nil {
			writeErr(w, 500, fmt.Errorf("saved, but could not apply to the active monitor: %w", err))
			return
		}
	}
	writeJSON(w, 200, m)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	err := s.sess.Create(s.deviceID(r))
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

// listOutputs is display.Outputs, replaceable in tests.
var listOutputs = display.Outputs

// displays lists this PC's displays and whether each stays on during a session.
func (s *Server) displays(w http.ResponseWriter, _ *http.Request) {
	type entry struct {
		display.Output
		Keep bool `json:"keep"`
	}
	outs, err := listOutputs()
	if errors.Is(err, display.ErrNotImplemented) {
		writeJSON(w, 200, map[string]any{"supported": false, "displays": []entry{}})
		return
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	keep := map[string]bool{}
	for _, id := range s.cfg.KeepDisplays() {
		keep[id] = true
	}
	list := make([]entry, 0, len(outs))
	for _, o := range outs {
		list = append(list, entry{o, keep[o.ID]})
	}
	writeJSON(w, 200, map[string]any{"supported": true, "displays": list})
}

func (s *Server) putKeep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keep []string `json:"keep"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.cfg.SetKeepDisplays(body.Keep); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.displays(w, r)
}

func (s *Server) destroy(w http.ResponseWriter, _ *http.Request) {
	if err := s.sess.Destroy(); err != nil {
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
		if r.Method == http.MethodGet && r.URL.Path == "/api/discover" {
			next.ServeHTTP(w, r)
			return
		}
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

// session lets a paired client report that its Remote Play stream started or stopped.
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := s.deviceID(r)
	switch body.State {
	case "start":
		created, err := s.sess.Start(id)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"monitorActive": s.disp.Active(), "created": created})
	case "stop":
		s.sess.Stop(id)
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErr(w, 400, errors.New(`state must be "start" or "stop"`))
	}
}

// wake sends a Wake-on-LAN packet on this host's networks for a paired device, so
// a device away from home (where broadcasts don't cross its VPN) can wake another PC.
func (s *Server) wake(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MAC string `json:"mac"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	mac, err := wol.ParseMAC(body.MAC)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := wol.Send(mac); err != nil {
		writeErr(w, 500, err)
		return
	}
	log.Printf("wake: sent magic packet for %s", mac)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// power shuts the PC down for a paired device (Decky plugin's Shut down button).
func (s *Server) power(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if body.Action != "shutdown" {
		writeErr(w, 400, errors.New(`action must be "shutdown"`))
		return
	}
	if err := s.shutdown(); err != nil {
		writeErr(w, 500, err)
		return
	}
	log.Printf("power: shutting down for device %q", s.deviceID(r))
	writeJSON(w, 200, map[string]bool{"ok": true})
}
