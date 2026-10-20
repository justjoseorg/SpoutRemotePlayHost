// Package picker opens the desktop's own "open file" dialog on this PC, so the web UI (which a
// browser can't give real file paths) can let the user choose a program.
package picker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrBusy is returned while another dialog from this host is still open.
var ErrBusy = errors.New("a file dialog is already open on this PC")

var busy sync.Mutex

// OpenFile shows a file dialog starting near start (a file or folder; may be empty) and
// returns the chosen path, or "" if the user cancelled.
func OpenFile(ctx context.Context, start string) (string, error) {
	if !busy.TryLock() {
		return "", ErrBusy
	}
	defer busy.Unlock()
	return openFile(ctx, startDir(start))
}

// startDir returns the folder to open the dialog in, or "" to let the desktop decide.
func startDir(start string) string {
	if start == "" {
		return ""
	}
	if st, err := os.Stat(start); err == nil && st.IsDir() {
		return start
	}
	if d := filepath.Dir(start); d != "." {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}
