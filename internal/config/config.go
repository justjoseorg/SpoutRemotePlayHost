package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Monitor holds the virtual monitor properties edited from the web UI.
type Monitor struct {
	Width     int `json:"width"`
	Height    int `json:"height"`
	RefreshHz int `json:"refreshHz"`
	// AutoCreate creates the monitor when a Remote Play session starts.
	AutoCreate bool `json:"autoCreate"`
	// Codec is unused (Steam negotiates the real codec); kept so older clients that send it still work.
	Codec string `json:"codec"`
}

func Default() Monitor {
	return Monitor{Width: 1920, Height: 1080, RefreshHz: 60, AutoCreate: true, Codec: "auto"}
}

func (m Monitor) Validate() error {
	switch {
	case m.Codec != "" && m.Codec != "auto" && m.Codec != "h264" && m.Codec != "hevc" && m.Codec != "av1":
		return fmt.Errorf("codec must be auto, h264, hevc or av1, got %q", m.Codec)
	case m.Width < 640 || m.Width > 7680:
		return fmt.Errorf("width must be 640-7680, got %d", m.Width)
	case m.Height < 480 || m.Height > 4320:
		return fmt.Errorf("height must be 480-4320, got %d", m.Height)
	case m.RefreshHz < 24 || m.RefreshHz > 240:
		return fmt.Errorf("refreshHz must be 24-240, got %d", m.RefreshHz)
	}
	return nil
}

// Store persists the monitor config as JSON.
type Store struct {
	mu      sync.Mutex
	path    string
	cur     Monitor
	devices map[string]Monitor
	keep    []string
}

// file is the on-disk layout: the default monitor inline plus one monitor config per paired device.
type file struct {
	Monitor
	Devices map[string]Monitor `json:"devices,omitempty"`
	// KeepDisplays lists physical displays (by ID) left on during a session; all others are turned off.
	KeepDisplays []string `json:"keepDisplays,omitempty"`
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, cur: Default(), devices: map[string]Monitor{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	f := file{Monitor: Default()}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := f.Monitor.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	for id, m := range f.Devices {
		if err := m.Validate(); err != nil {
			return nil, fmt.Errorf("invalid %s device %s: %w", path, id, err)
		}
		s.devices[id] = m
	}
	s.cur, s.keep = f.Monitor, f.KeepDisplays
	return s, nil
}

// KeepDisplays returns the IDs of physical displays that stay on during a session.
func (s *Store) KeepDisplays() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keep...)
}

// SetKeepDisplays saves the IDs of physical displays that stay on during a session.
func (s *Store) SetKeepDisplays(ids []string) error {
	if len(ids) > 32 {
		return errors.New("too many displays")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.keep
	s.keep = append([]string(nil), ids...)
	if err := s.saveLocked("", nil); err != nil {
		s.keep = old
		return err
	}
	return nil
}

func (s *Store) Get() Monitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Set updates the default monitor config.
func (s *Store) Set(m Monitor) error { return s.SetFor("", m) }

// GetFor returns the monitor config of a paired device, falling back to the default when it has none.
func (s *Store) GetFor(id string) Monitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.devices[id]; ok && id != "" {
		return m
	}
	return s.cur
}

// Device returns a device's own config and whether it has one (otherwise it uses the default).
func (s *Store) Device(id string) (Monitor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.devices[id]
	return m, ok
}

// SetFor saves the monitor config of a paired device; an empty id sets the default.
func (s *Store) SetFor(id string, m Monitor) error {
	if err := m.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(id, &m)
}

// Forget removes a device's config, e.g. when it is unpaired.
func (s *Store) Forget(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; !ok {
		return nil
	}
	return s.saveLocked(id, nil)
}

// saveLocked applies the change (nil deletes a device; nil with id "" only rewrites the file)
// and writes the file; memory changes only on success.
func (s *Store) saveLocked(id string, m *Monitor) error {
	f := file{Monitor: s.cur, Devices: map[string]Monitor{}, KeepDisplays: s.keep}
	for k, v := range s.devices {
		f.Devices[k] = v
	}
	switch {
	case m == nil && id == "":
	case m == nil:
		delete(f.Devices, id)
	case id == "":
		f.Monitor = *m
	default:
		f.Devices[id] = *m
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.cur, s.devices = f.Monitor, f.Devices
	return nil
}

// DefaultPath returns the per-user config file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "SpoutRemotePlayHost", "config.json"), nil
}

// LoadOrCreateToken returns the API token stored next to the config, creating it on first use.
func LoadOrCreateToken(configPath string) (string, error) {
	path := filepath.Join(filepath.Dir(configPath), "token")
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return string(data), nil
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return tok, os.WriteFile(path, []byte(tok), 0o600)
}
