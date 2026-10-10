// Package signin lets a paired device type its owner's PIN into the Windows sign-in screen,
// so a PC woken with Wake-on-LAN can be signed in remotely and Steam can start.
//
// It runs as an optional system service separate from the host, because the host only runs
// once someone is signed in. The PIN is typed once and never stored or logged.
package signin

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DefaultListen is where the sign-in service listens; the host itself uses 47995.
const DefaultListen = "0.0.0.0:47994"

var (
	// ErrNotSignInScreen means the PC is not at the sign-in or lock screen, so nothing was typed.
	ErrNotSignInScreen = errors.New("this PC is not at the sign-in screen")
	errBadPIN          = errors.New("the PIN must be 4 to 32 digits")
)

var pinRE = regexp.MustCompile(`^[0-9]{4,32}$`)

const (
	maxTries = 5
	window   = 5 * time.Minute
)

// Typer types pin into the sign-in screen, or returns ErrNotSignInScreen.
type Typer func(pin string) error

// Handler serves the sign-in API. valid reports whether a bearer token belongs to a paired device.
type Handler struct {
	valid func(token string) bool
	typ   Typer
	now   func() time.Time

	mu     sync.Mutex
	tries  []time.Time
	typing sync.Mutex
}

func NewHandler(valid func(token string) bool, typ Typer) *Handler {
	return &Handler{valid: valid, typ: typ, now: time.Now}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/signin" {
		http.NotFound(w, r)
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || !h.valid(auth[7:]) {
		writeErr(w, http.StatusUnauthorized, errors.New("pair this device with Spout Remote Play Host first"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodPost:
		h.post(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) post(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !pinRE.MatchString(body.PIN) {
		writeErr(w, http.StatusBadRequest, errBadPIN)
		return
	}
	if !h.allow() {
		writeErr(w, http.StatusTooManyRequests, errors.New("too many sign-in attempts; wait a few minutes"))
		return
	}
	// Interleaved key presses from two attempts would count as wrong PINs.
	if !h.typing.TryLock() {
		writeErr(w, http.StatusConflict, errors.New("a sign-in is already in progress"))
		return
	}
	err := h.typ(body.PIN)
	h.typing.Unlock()
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, ErrNotSignInScreen) {
			code = http.StatusConflict
		}
		writeErr(w, code, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// allow limits attempts across all devices, on top of Windows' own PIN lockout.
func (h *Handler) allow() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	kept := h.tries[:0]
	for _, t := range h.tries {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	h.tries = kept
	if len(h.tries) >= maxTries {
		return false
	}
	h.tries = append(h.tries, now)
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
