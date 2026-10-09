//go:build windows

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// The release exe is built with -H=windowsgui, so there is no console: also log to a file
// and show fatal errors in a message box instead of exiting silently.
func setupLog(dir string) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	p := filepath.Join(dir, "spout-host.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 1<<20 {
		_ = os.Rename(p, p+".old")
	}
	// Append: a second launch (which only opens the UI) must not wipe the running host's log.
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	// File first: MultiWriter stops at the first error, and a GUI exe has no valid stderr.
	log.SetOutput(io.MultiWriter(f, os.Stderr))
}

func fatal(err error) {
	log.Println("fatal:", err)
	title, _ := windows.UTF16PtrFromString("Spout Remote Play Host")
	msg, _ := windows.UTF16PtrFromString(err.Error())
	windows.MessageBox(0, msg, title, windows.MB_OK|windows.MB_ICONERROR)
	os.Exit(1)
}
