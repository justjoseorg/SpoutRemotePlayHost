// Package apps stores the programs a user wants available in Steam's library on this PC.
package apps

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// App is one program exposed through Steam as a non-Steam shortcut.
type App struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Exe        string `json:"exe"`
	Args       string `json:"args,omitempty"`
	StartDir   string `json:"startDir,omitempty"`
	SteamAppID uint32 `json:"steamAppId,omitempty"`
}

// ErrNotFound is returned for an unknown app id.
var ErrNotFound = errors.New("app not found")

// Validate checks the user-supplied fields.
func (a App) Validate() error {
	switch {
	case strings.TrimSpace(a.Name) == "" || len(a.Name) > 80:
		return errors.New("name must be 1-80 characters")
	case strings.TrimSpace(a.Exe) == "" || len(a.Exe) > 1024:
		return errors.New("exe is required (max 1024 characters)")
	case len(a.Args) > 2048 || len(a.StartDir) > 1024:
		return errors.New("args or startDir too long")
	}
	for _, s := range []string{a.Name, a.Exe, a.Args, a.StartDir} {
		if strings.ContainsAny(s, "\x00\r\n") {
			return errors.New("fields must not contain control characters")
		}
	}
	return nil
}

// Store persists the catalog as JSON.
type Store struct {
	mu   sync.Mutex
	path string
	list []App
}

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) List() []App {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]App(nil), s.list...)
}

func (s *Store) Get(id string) (App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.list {
		if a.ID == id {
			return a, nil
		}
	}
	return App{}, ErrNotFound
}

// Add validates and stores a new app, assigning its id.
func (s *Store) Add(a App) (App, error) {
	if err := a.Validate(); err != nil {
		return App{}, err
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return App{}, err
	}
	a.ID, a.SteamAppID = hex.EncodeToString(b[:]), 0
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append(s.list, a)
	if err := s.save(); err != nil {
		s.list = s.list[:len(s.list)-1]
		return App{}, err
	}
	return a, nil
}

// SetSteamAppID records (or clears, with 0) the Steam shortcut id for an app.
func (s *Store) SetSteamAppID(id string, steamID uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.list {
		if s.list[i].ID == id {
			old := s.list[i].SteamAppID
			s.list[i].SteamAppID = steamID
			if err := s.save(); err != nil {
				s.list[i].SteamAppID = old
				return err
			}
			return nil
		}
	}
	return ErrNotFound
}

func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.list {
		if a.ID == id {
			old := s.list
			s.list = append(append([]App(nil), old[:i]...), old[i+1:]...)
			if err := s.save(); err != nil {
				s.list = old
				return err
			}
			return nil
		}
	}
	return ErrNotFound
}
