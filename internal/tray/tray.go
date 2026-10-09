// Package tray shows a system tray icon that opens the web UI.
package tray

import (
	_ "embed"
	"log"
	"os/exec"
	"runtime"

	"fyne.io/systray"
)

//go:embed icon.png
var iconPNG []byte

//go:embed icon.ico
var iconICO []byte

// OpenURL opens url in the default browser.
func OpenURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Println("open UI:", err)
		return
	}
	go cmd.Wait()
}

// Run blocks on the calling (main) goroutine until Quit is chosen, then calls onQuit.
func Run(url, version string, onQuit func()) {
	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(iconICO)
		} else {
			systray.SetIcon(iconPNG)
		}
		systray.SetTitle("Spout Remote Play Host")
		systray.SetTooltip("Spout Remote Play Host")
		systray.SetOnTapped(func() { OpenURL(url) })

		open := systray.AddMenuItem("Open Spout Remote Play Host", "Open the web UI")
		systray.AddSeparator()
		ver := systray.AddMenuItem("Version "+version, "")
		ver.Disable()
		quit := systray.AddMenuItem("Quit", "Stop the host")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					OpenURL(url)
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, onQuit)
}
