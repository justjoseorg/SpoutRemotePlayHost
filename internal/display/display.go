package display

import "errors"

var ErrNotImplemented = errors.New("virtual display not implemented on this platform yet")

// Mode describes the virtual monitor requested for a streaming session.
type Mode struct {
	Width, Height, RefreshHz int
}

// Manager creates and removes the virtual monitor used for a session.
type Manager interface {
	Create(Mode) error
	Destroy() error
}

type stub struct{}

func (stub) Create(Mode) error { return ErrNotImplemented }
func (stub) Destroy() error    { return nil }

// New returns the Manager for the current OS.
func New() Manager { return stub{} }
