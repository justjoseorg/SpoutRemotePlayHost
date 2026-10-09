//go:build linux

package display

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const helperPath = "/usr/local/libexec/spout-vdisplay"

// runner executes a command and returns its combined output.
type runner func(name string, args ...string) (string, error)

func execRunner(name string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
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
