// Package notify shows a desktop notification on the host.
package notify

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const toastScript = `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType=WindowsRuntime] | Out-Null
$x = New-Object Windows.Data.Xml.Dom.XmlDocument
$x.LoadXml('<toast activationType="protocol"><visual><binding template="ToastGeneric"><text/><text/></binding></visual></toast>')
$t = $x.GetElementsByTagName('text')
$t.Item(0).InnerText = $env:SPOUT_TITLE
$t.Item(1).InnerText = $env:SPOUT_BODY
if ($env:SPOUT_URL) { $x.DocumentElement.SetAttribute('launch', $env:SPOUT_URL) }
$n = [Windows.UI.Notifications.ToastNotification]::new($x)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show($n)
`

// Send displays a notification; clicking it (or its action) opens url when non-empty.
// Text is passed via the environment/arguments, never interpolated into a script.
func Send(title, body, url string) {
	log.Printf("notify: %s: %s", title, body)
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", toastScript)
		cmd.Env = append(os.Environ(), "SPOUT_TITLE="+title, "SPOUT_BODY="+body, "SPOUT_URL="+url)
		go func() {
			if err := cmd.Run(); err != nil {
				log.Println("notification failed:", err)
			}
		}()
	case "linux":
		path, err := exec.LookPath("notify-send")
		if err != nil {
			return
		}
		args := []string{"--app-name=Spout Host", "--urgency=critical"}
		if url != "" {
			args = append(args, "--action=open=Open", "--wait")
		}
		args = append(args, "--", title, body)
		cmd := exec.Command(path, args...)
		go func() {
			out, err := cmd.Output()
			if err != nil {
				log.Println("notification failed:", err)
				return
			}
			// Any returned action (Open button or a click on the body) means the user wants the UI.
			action := strings.TrimSpace(string(out))
			if url == "" || action == "" {
				return
			}
			log.Printf("notification action %q: opening %s", action, url)
			xdg, err := exec.LookPath("xdg-open")
			if err != nil {
				log.Println("cannot open UI:", err)
				return
			}
			if err := exec.Command(xdg, url).Start(); err != nil {
				log.Println("xdg-open failed:", err)
			}
		}()
	}
}
