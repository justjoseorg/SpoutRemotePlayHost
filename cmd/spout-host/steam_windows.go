package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlib"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ensureSteam starts Steam (minimized to the tray) if it isn't running a little
// after sign-in, so a PC signed in remotely can be streamed from.
func ensureSteam(after time.Duration) {
	time.Sleep(after)
	if steamRunning() {
		return
	}
	exe := steamExe()
	if exe == "" {
		log.Println("Steam isn't running and steam.exe wasn't found")
		return
	}
	log.Println("Steam isn't running; starting", exe)
	if err := exec.Command(exe, "-silent").Start(); err != nil {
		log.Println("start Steam:", err)
	}
}

func steamExe() string {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		p, _, err := k.GetStringValue("SteamExe")
		k.Close()
		if err == nil && p != "" {
			if _, err := os.Stat(p); err == nil {
				return filepath.FromSlash(p)
			}
		}
	}
	for _, r := range steamlib.Roots() {
		if p := filepath.Join(r, "steam.exe"); fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func steamRunning() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return true // can't tell; don't start a second Steam
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "steam.exe") {
			return true
		}
	}
	return false
}
