// Package steamlog detects Remote Play sessions by tailing Steam's streaming_log.txt.
package steamlog

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

type Event struct {
	Start  bool
	Client string // Steam's name for the streaming client; empty on stop
}

var (
	startRe = regexp.MustCompile(`Streaming started to (.+?) at \S+:\d+`)
	// Steam on Linux tears down its PipeWire capture when a session ends; on Windows it logs
	// "Encoding complete" once, when the session's encoder shuts down.
	stopRe = regexp.MustCompile(`PipeWire: Deinitializing streaming|\] Encoding complete\s*$`)
)

// Parse maps one log line to an event.
func Parse(line string) (Event, bool) {
	if m := startRe.FindStringSubmatch(line); m != nil {
		return Event{Start: true, Client: m[1]}, true
	}
	if stopRe.MatchString(line) {
		return Event{}, true
	}
	return Event{}, false
}

// Candidates lists likely locations of Steam's streaming log.
func Candidates() []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		for _, e := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if p := os.Getenv(e); p != "" {
				dirs = append(dirs, filepath.Join(p, "Steam", "logs"))
			}
		}
	} else if h, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(h, ".local/share/Steam/logs"),
			filepath.Join(h, ".steam/steam/logs"),
			filepath.Join(h, ".var/app/com.valvesoftware.Steam/.local/share/Steam/logs"),
		)
	}
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = filepath.Join(d, "streaming_log.txt")
	}
	return out
}

// Watch tails the log (starting at its current end) and calls fn for each session event
// until stop is closed. It re-resolves the file so Steam restarts and rotation are tolerated.
func Watch(paths []string, poll time.Duration, stop <-chan struct{}, fn func(Event)) {
	var f *os.File
	var path string
	var pos int64
	var pending []byte
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if f == nil {
			for _, p := range paths {
				if fi, err := os.Stat(p); err == nil {
					if ff, err := os.Open(p); err == nil {
						f, path, pos, pending = ff, p, fi.Size(), nil
						break
					}
				}
			}
			if f == nil {
				continue
			}
		}
		fi, err := os.Stat(path)
		if err != nil || fi.Size() < pos {
			// Truncated or replaced: reopen from the start of the new file.
			f.Close()
			f = nil
			if err == nil {
				if ff, err := os.Open(path); err == nil {
					f, pos, pending = ff, 0, nil
				}
			}
			continue
		}
		if fi.Size() == pos {
			continue
		}
		buf := make([]byte, fi.Size()-pos)
		n, _ := f.ReadAt(buf, pos)
		if n == 0 {
			continue
		}
		pos += int64(n)
		pending = append(pending, buf[:n]...)
		for {
			i := bytes.IndexByte(pending, '\n')
			if i < 0 {
				break
			}
			if ev, ok := Parse(string(pending[:i])); ok {
				fn(ev)
			}
			pending = pending[i+1:]
		}
	}
}
