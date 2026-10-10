//go:build !windows

package power

func shutdownCmd() (string, []string) {
	return "systemctl", []string{"poweroff"}
}
