// Package hotkey listens for Moonlight's "quit stream" shortcut (Ctrl+Alt+Shift+Q)
// on the host so the virtual monitor can be closed and control returned to the
// physical displays.
package hotkey

import "errors"

var ErrUnsupported = errors.New("global hotkey not supported on this platform yet")

// Listen blocks until stop is closed, calling onTrigger each time the shortcut is pressed.
// It returns ErrUnsupported (immediately) where there is no implementation.
func Listen(stop <-chan struct{}, onTrigger func()) error {
	return listen(stop, onTrigger)
}
