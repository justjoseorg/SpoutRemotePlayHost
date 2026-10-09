package display

import (
	"errors"
	"sync"
)

var ErrNotImplemented = errors.New("virtual display driver not implemented on this platform yet")

// Mode describes the virtual monitor requested for a streaming session.
type Mode struct {
	Width, Height, RefreshHz int
}

// Manager creates and removes the virtual monitor used for a session.
type Manager interface {
	Create(Mode) error
	Destroy() error
	Active() bool
}

// backend is the OS-specific driver layer.
type backend interface {
	create(Mode) error
	destroy() error
}

type manager struct {
	mu     sync.Mutex
	b      backend
	active bool
}

func (m *manager) Create(mode Mode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		if err := m.b.destroy(); err != nil {
			return err
		}
		m.active = false
	}
	if err := m.b.create(mode); err != nil {
		return err
	}
	m.active = true
	return nil
}

func (m *manager) Destroy() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return nil
	}
	if err := m.b.destroy(); err != nil {
		return err
	}
	m.active = false
	return nil
}

func (m *manager) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

type stub struct{}

func (stub) create(Mode) error { return ErrNotImplemented }
func (stub) destroy() error    { return nil }

// New returns the Manager for the current OS.
func New() Manager { return &manager{b: newBackend()} }

func newWithBackend(b backend) Manager { return &manager{b: b} }
