//go:build !windows

package main

import "time"

// ensureSteam is Windows-only; on Linux, Steam is started by the desktop session.
func ensureSteam(time.Duration) {}
