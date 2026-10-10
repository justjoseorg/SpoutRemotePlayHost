// Package power shuts the PC down for a paired device.
package power

import "os/exec"

// Shutdown starts a normal (not forced) shutdown. Apps with unsaved work may still
// ask before closing.
func Shutdown() error {
	name, args := shutdownCmd()
	return exec.Command(name, args...).Start()
}
