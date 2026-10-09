//go:build !windows

package display

func newBackend() backend { return stub{} }
