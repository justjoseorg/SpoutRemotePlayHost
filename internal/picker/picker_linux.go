//go:build linux

package picker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// openFile uses KDE's kdialog, or zenity on other desktops. Both print the chosen path and
// exit with status 1 when the user cancels.
func openFile(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	const title = "Choose a program for Steam"
	var cmd *exec.Cmd
	if p, err := exec.LookPath("kdialog"); err == nil {
		cmd = exec.CommandContext(ctx, p, "--title", title, "--getopenfilename", dir)
	} else if p, err := exec.LookPath("zenity"); err == nil {
		cmd = exec.CommandContext(ctx, p, "--file-selection", "--title="+title, "--filename="+strings.TrimSuffix(dir, "/")+"/")
	} else {
		return "", errors.New("no file dialog found: install kdialog or zenity, or type the path")
	}
	cmd.Env = desktopEnv(os.Environ())
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// desktopEnv lets the dialog reach the desktop when the service started before the session
// exported its display variables.
func desktopEnv(env []string) []string {
	get := func(k string) string {
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, k+"="); ok {
				return v
			}
		}
		return ""
	}
	if get("WAYLAND_DISPLAY") != "" || get("DISPLAY") != "" {
		return env
	}
	run := get("XDG_RUNTIME_DIR")
	if run == "" {
		run = fmt.Sprintf("/run/user/%d", os.Getuid())
		env = append(env, "XDG_RUNTIME_DIR="+run)
	}
	socks, _ := filepath.Glob(filepath.Join(run, "wayland-[0-9]*"))
	for _, s := range socks {
		if !strings.HasSuffix(s, ".lock") {
			return append(env, "WAYLAND_DISPLAY="+filepath.Base(s), "QT_QPA_PLATFORM=wayland")
		}
	}
	return env
}
