//go:build !windows

package hotkey

func listen(<-chan struct{}, func()) error { return ErrUnsupported }
