//go:build !windows && !linux

package display

func newBackend() backend { return stub{} }
