package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
)

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return err == nil && ip != nil && ip.IsLoopback()
}

func localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) {
			writeErr(w, http.StatusForbidden, errors.New("only available on this PC"))
			return
		}
		next(w, r)
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

// hostInfo describes this PC for the client: its name and the MAC of the NIC that served the request,
// so the client can add the PC to its Wake-on-LAN list without the user typing a MAC.
func hostInfo(r *http.Request) map[string]string {
	info := map[string]string{}
	if name, err := os.Hostname(); err == nil {
		info["hostname"] = name
	}
	local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	tcp, ok := local.(*net.TCPAddr)
	if !ok {
		return info
	}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(tcp.IP) && len(ifc.HardwareAddr) == 6 {
				info["mac"] = strings.ToUpper(ifc.HardwareAddr.String())
				return info
			}
		}
	}
	return info
}

func (s *Server) pairRequest(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name       string               `json:"name"`
		Salt       string               `json:"salt"`
		PINHash    string               `json:"pinHash"`
		SecretHash string               `json:"secretHash"`
		Caps       pairing.Capabilities `json:"caps"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	id, err := s.pair.Request(b.Name, b.Salt, b.PINHash, b.SecretHash, b.Caps)
	switch {
	case errors.Is(err, pairing.ErrBusy):
		writeErr(w, http.StatusTooManyRequests, err)
	case err != nil:
		writeErr(w, http.StatusBadRequest, err)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"id": id})
	}
}

func (s *Server) pairPoll(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	status, token := s.pair.Poll(b.ID, b.Secret)
	out := map[string]string{"status": status}
	if status == "approved" {
		out["token"] = token
		for k, v := range hostInfo(r) {
			out[k] = v
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pairPending(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.pair.Pending())
}

func (s *Server) pairConfirm(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ID  string `json:"id"`
		PIN string `json:"pin"`
	}
	if !decodeBody(w, r, &b) {
		return
	}
	switch err := s.pair.Confirm(b.ID, b.PIN); {
	case errors.Is(err, pairing.ErrWrongPIN), errors.Is(err, pairing.ErrTooMany):
		writeErr(w, http.StatusForbidden, err)
	case errors.Is(err, pairing.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"paired": true})
	}
}

func (s *Server) pairDeny(w http.ResponseWriter, r *http.Request) {
	s.pair.Deny(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) clients(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.pair.Clients())
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if err := s.pair.Revoke(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	_ = s.cfg.Forget(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) knownClient(id string) bool {
	for _, c := range s.pair.Clients() {
		if c.ID == id {
			return true
		}
	}
	return false
}

// clientConfig reports a device's monitor config and whether it is custom or inherited from the defaults.
func (s *Server) clientConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.knownClient(id) {
		writeErr(w, http.StatusNotFound, pairing.ErrUnknownPeer)
		return
	}
	_, custom := s.cfg.Device(id)
	writeJSON(w, http.StatusOK, map[string]any{"custom": custom, "config": s.cfg.GetFor(id)})
}

func (s *Server) putClientConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.knownClient(id) {
		writeErr(w, http.StatusNotFound, pairing.ErrUnknownPeer)
		return
	}
	var m config.Monitor
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	old := s.cfg.GetFor(id)
	if err := s.cfg.SetFor(id, m); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if s.sess.OwnedBy(id) && (old.Width != m.Width || old.Height != m.Height || old.RefreshHz != m.RefreshHz) {
		if err := s.sess.Reapply(id); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"custom": true, "config": m})
}

func (s *Server) resetClientConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.cfg.Forget(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"custom": false, "config": s.cfg.GetFor(id)})
}

// discover lets clients find this host on the LAN. It reveals only the app name, hostname and version.
func (s *Server) discover(w http.ResponseWriter, _ *http.Request) {
	name, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]string{"app": "spout-host", "name": name, "version": s.version})
}
