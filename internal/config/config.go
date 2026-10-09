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
	// Codec is a preference hint (auto, h264, hevc, av1); Steam Remote Play negotiates the real codec itself.
	Codec string `json:"codec"`
}

func Default() Monitor {
	return Monitor{Width: 1280, Height: 800, RefreshHz: 60, AutoCreate: true, Codec: "auto"}
}

func (m Monitor) Validate() error {
	switch {
	case m.Codec != "auto" && m.Codec != "h264" && m.Codec != "hevc" && m.Codec != "av1":
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
	mu   sync.Mutex
	path string
	cur  Monitor
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, cur: Default()}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.cur); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.cur.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Get() Monitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *Store) Set(m Monitor) error {
	if err := m.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(m, "", "  ")
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
	s.cur = m
	return nil
}

// DefaultPath returns the per-user config file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "SpigotRemotePlayHost", "config.json"), nil
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
