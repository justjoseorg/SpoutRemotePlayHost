// Package pairing implements PIN-based client pairing.
//
// The client generates a 4-digit PIN, shows it to the user and sends the host only a salted hash of it
// plus a hash of a private poll secret. The host raises a notification; the user types the PIN into the
// host UI (loopback only). On a match the host issues a per-client API token, which the client collects
// by proving knowledge of its poll secret.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	requestTTL  = 2 * time.Minute
	maxAttempts = 5
	maxPending  = 3
)

var (
	ErrNotFound    = errors.New("no such pairing request")
	ErrWrongPIN    = errors.New("wrong PIN")
	ErrTooMany     = errors.New("too many wrong attempts; request cancelled")
	ErrBusy        = errors.New("too many pending pairing requests")
	ErrBadRequest  = errors.New("invalid pairing request")
	ErrUnknownPeer = errors.New("unknown client")
)

// Capabilities is what a client reports about itself when pairing; all fields are optional hints.
type Capabilities struct {
	HEVC      bool `json:"hevc"`
	AV1       bool `json:"av1"`
	Width     int  `json:"width,omitempty"`
	Height    int  `json:"height,omitempty"`
	RefreshHz int  `json:"refreshHz,omitempty"`
}

func (c Capabilities) sane() Capabilities {
	if c.Width < 0 || c.Width > 16384 || c.Height < 0 || c.Height > 16384 {
		c.Width, c.Height = 0, 0
	}
	if c.RefreshHz < 0 || c.RefreshHz > 1000 {
		c.RefreshHz = 0
	}
	return c
}

// Client is a paired device. Only a hash of its token is stored.
type Client struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	TokenHash string       `json:"tokenHash,omitempty"`
	Added     time.Time    `json:"added"`
	Caps      Capabilities `json:"caps"`
}

// PendingInfo is what the host UI may show about a waiting request. The PIN hash is never exposed.
type PendingInfo struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Expires time.Time `json:"expires"`
}

type request struct {
	id         string
	name       string
	caps       Capabilities
	salt       string
	pinHash    []byte
	secretHash []byte
	expires    time.Time
	attempts   int
	token      string // set once approved, handed out a single time
}

type Manager struct {
	// URL is opened when the pairing notification is clicked.
	URL     string
	mu      sync.Mutex
	path    string
	clients []Client
	pending map[string]*request
	notify  func(title, body, url string)
	now     func() time.Time
}

// New loads paired clients from path (may be empty for in-memory use). notify may be nil.
func New(path string, notify func(title, body, url string)) (*Manager, error) {
	m := &Manager{path: path, pending: map[string]*request{}, notify: notify, now: time.Now}
	if path == "" {
		return m, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &m.clients); err != nil {
		return nil, err
	}
	return m, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashHex(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// PINHash is the value a client sends in place of the PIN.
func PINHash(salt, pin string) string { return hex.EncodeToString(hashHex(salt + pin)) }

// SecretHash is the value a client sends in place of its poll secret.
func SecretHash(secret string) string { return hex.EncodeToString(hashHex(secret)) }

func cleanName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r != 0x7f && r != '<' && r != '>' && r != '&' && r != '"' && r != '\'' {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 40 {
		out = out[:40]
	}
	if out == "" {
		out = "Unknown device"
	}
	return out
}

func (m *Manager) purge() {
	now := m.now()
	for id, r := range m.pending {
		if now.After(r.expires) {
			delete(m.pending, id)
		}
	}
}

// Request registers a pairing attempt and notifies the host user.
func (m *Manager) Request(name, salt, pinHashHex, secretHashHex string, caps Capabilities) (string, error) {
	pinHash, err1 := hex.DecodeString(pinHashHex)
	secretHash, err2 := hex.DecodeString(secretHashHex)
	if err1 != nil || err2 != nil || len(pinHash) != 32 || len(secretHash) != 32 || len(salt) < 8 || len(salt) > 64 {
		return "", ErrBadRequest
	}
	m.mu.Lock()
	m.purge()
	if len(m.pending) >= maxPending {
		m.mu.Unlock()
		return "", ErrBusy
	}
	r := &request{
		id: randHex(8), name: cleanName(name), caps: caps.sane(), salt: salt,
		pinHash: pinHash, secretHash: secretHash, expires: m.now().Add(requestTTL),
	}
	m.pending[r.id] = r
	notify, url := m.notify, m.URL
	m.mu.Unlock()
	if notify != nil {
		notify("Incoming pairing request", r.name+" wants to pair. Click to enter the PIN shown on that device.", url)
	}
	return r.id, nil
}

func (m *Manager) Pending() []PendingInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purge()
	out := []PendingInfo{}
	for _, r := range m.pending {
		if r.token == "" {
			out = append(out, PendingInfo{ID: r.id, Name: r.name, Expires: r.expires})
		}
	}
	return out
}

// Confirm checks the PIN typed on the host and, on a match, creates the client and its token.
func (m *Manager) Confirm(id, pin string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purge()
	r, ok := m.pending[id]
	if !ok || r.token != "" {
		return ErrNotFound
	}
	if subtle.ConstantTimeCompare(hashHex(r.salt+strings.TrimSpace(pin)), r.pinHash) != 1 {
		r.attempts++
		if r.attempts >= maxAttempts {
			delete(m.pending, id)
			return ErrTooMany
		}
		return ErrWrongPIN
	}
	token := randHex(24)
	m.clients = append(m.clients, Client{
		ID: randHex(4), Name: r.name, Caps: r.caps, TokenHash: hex.EncodeToString(hashHex(token)), Added: m.now().UTC(),
	})
	if err := m.saveLocked(); err != nil {
		m.clients = m.clients[:len(m.clients)-1]
		return err
	}
	r.token = token
	if m.notify != nil {
		go m.notify("Device paired", r.name+" is now paired with this PC.", "")
	}
	return nil
}

func (m *Manager) Deny(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pending, id)
}

// Poll lets the requesting client collect its token once approved. Status is "pending", "approved" or "gone".
func (m *Manager) Poll(id, secret string) (status, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purge()
	r, ok := m.pending[id]
	if !ok || subtle.ConstantTimeCompare(hashHex(secret), r.secretHash) != 1 {
		return "gone", ""
	}
	if r.token == "" {
		return "pending", ""
	}
	delete(m.pending, id)
	return "approved", r.token
}

// Valid reports whether token belongs to a paired client.
func (m *Manager) Valid(token string) bool {
	if token == "" {
		return false
	}
	want := []byte(hex.EncodeToString(hashHex(token)))
	m.mu.Lock()
	defer m.mu.Unlock()
	ok := 0
	for _, c := range m.clients {
		ok |= subtle.ConstantTimeCompare([]byte(c.TokenHash), want)
	}
	return ok == 1
}

// ClientID returns the id of the paired client owning token, or "" if none.
func (m *Manager) ClientID(token string) string {
	if token == "" {
		return ""
	}
	want := []byte(hex.EncodeToString(hashHex(token)))
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		if subtle.ConstantTimeCompare([]byte(c.TokenHash), want) == 1 {
			return c.ID
		}
	}
	return ""
}

// Clients lists paired devices without their token hashes.
func (m *Manager) Clients() []Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Client, len(m.clients))
	for i, c := range m.clients {
		c.TokenHash = ""
		out[i] = c
	}
	return out
}

func (m *Manager) Revoke(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.clients {
		if c.ID == id {
			old := m.clients
			m.clients = append(append([]Client{}, old[:i]...), old[i+1:]...)
			if err := m.saveLocked(); err != nil {
				m.clients = old
				return err
			}
			return nil
		}
	}
	return ErrUnknownPeer
}

func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(m.clients, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}
