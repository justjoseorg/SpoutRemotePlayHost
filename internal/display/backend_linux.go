//go:build linux

package display

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const helperPath = "/usr/local/libexec/spout-vdisplay"

// runner executes a command and returns its combined output.
type runner func(name string, args ...string) (string, error)

func execRunner(name string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Env = sessionEnv(os.Environ())
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// sessionEnv makes sure Wayland tools can reach the compositor. The service may start before
// the desktop session exports its environment, and Qt aborts without a Wayland display.
func sessionEnv(env []string) []string {
	has := func(k string) bool {
		for _, e := range env {
			if strings.HasPrefix(e, k+"=") {
				return true
			}
		}
		return false
	}
	if !has("XDG_RUNTIME_DIR") {
		env = append(env, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", os.Getuid()))
	}
	if !has("WAYLAND_DISPLAY") {
		dir := ""
		for _, e := range env {
			if strings.HasPrefix(e, "XDG_RUNTIME_DIR=") {
				dir = strings.TrimPrefix(e, "XDG_RUNTIME_DIR=")
			}
		}
		socks, _ := filepath.Glob(filepath.Join(dir, "wayland-[0-9]*"))
		for _, sock := range socks {
			if !strings.HasSuffix(sock, ".lock") {
				env = append(env, "WAYLAND_DISPLAY="+filepath.Base(sock))
				break
			}
		}
	}
	if !has("QT_QPA_PLATFORM") {
		env = append(env, "QT_QPA_PLATFORM=wayland")
	}
	return env
}

// vdisplay drives the vibeshine_drm kernel module through a root helper (sudoers
// allows only that helper) and then applies the mode to the KWin output with kscreen-doctor.
type vdisplay struct {
	run    runner
	sleep  func(time.Duration)
	output string
}

func newBackend() backend { return &vdisplay{run: execRunner, sleep: time.Sleep} }

func (v *vdisplay) create(m Mode) error {
	out, err := v.run("sudo", "-n", helperPath, "create",
		fmt.Sprint(m.Width), fmt.Sprint(m.Height), fmt.Sprint(m.RefreshHz))
	if err != nil {
		return fmt.Errorf("virtual display driver: %w: %s (run the Linux installer to set up the driver)", err, out)
	}
	name := lastLine(out)
	if name == "" {
		return fmt.Errorf("virtual display helper returned no connector name")
	}
	v.output = name

	// KWin picks up the hotplug asynchronously; the output may not be listed for a moment.
	arg := fmt.Sprintf("output.%s.mode.%dx%d@%d", name, m.Width, m.Height, m.RefreshHz)
	var last string
	for i := 0; i < 20; i++ {
		last, err = v.run("kscreen-doctor", fmt.Sprintf("output.%s.enable", name), arg)
		if err == nil {
			return nil
		}
		v.sleep(250 * time.Millisecond)
	}
	_, _ = v.run("sudo", "-n", helperPath, "destroy")
	v.output = ""
	return fmt.Errorf("could not apply %dx%d@%d to %s: %s", m.Width, m.Height, m.RefreshHz, name, last)
}

func (v *vdisplay) destroy() error {
	out, err := v.run("sudo", "-n", helperPath, "destroy")
	if err != nil {
		return fmt.Errorf("destroy virtual display: %w: %s", err, out)
	}
	v.output = ""
	return nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
