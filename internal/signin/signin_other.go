//go:build !windows

package signin

import "errors"

// RunService is Windows-only: other systems have no sign-in screen the host could type into.
func RunService(string, string) error {
	return errors.New("the sign-in service is only available on Windows")
}

// TypeFromStdin is Windows-only.
func TypeFromStdin() int { return 4 }
