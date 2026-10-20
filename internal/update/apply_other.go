//go:build !windows && !linux

package update

import "errors"

func apply(string) error { return errors.New("updates are not supported on this system") }

// RunSetup is only used on Windows.
func RunSetup(string) error { return errors.New("not supported on this system") }
