//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	modAlt      = 0x1
	modControl  = 0x2
	modShift    = 0x4
	modNoRepeat = 0x4000
	vkQ         = 0x51
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
	hotkeyID    = 1
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	registerHotKey    = user32.NewProc("RegisterHotKey")
	unregisterHotKey  = user32.NewProc("UnregisterHotKey")
	getMessage        = user32.NewProc("GetMessageW")
	postThreadMessage = user32.NewProc("PostThreadMessageW")
)

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func listen(stop <-chan struct{}, onTrigger func()) error {
	ready := make(chan error, 1)
	threadID := make(chan uint32, 1)
	done := make(chan struct{})

	// Hotkeys are bound to the registering thread's message queue.
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if r, _, err := registerHotKey.Call(0, hotkeyID, modControl|modAlt|modShift|modNoRepeat, vkQ); r == 0 {
			ready <- fmt.Errorf("RegisterHotKey Ctrl+Alt+Shift+Q: %w", err)
			return
		}
		defer unregisterHotKey.Call(0, hotkeyID)
		threadID <- windows.GetCurrentThreadId()
		ready <- nil

		var m msg
		for {
			r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			if m.Message == wmHotkey && m.WParam == hotkeyID {
				onTrigger()
			}
		}
	}()

	if err := <-ready; err != nil {
		<-done
		return err
	}
	tid := <-threadID
	<-stop
	postThreadMessage.Call(uintptr(tid), wmQuit, 0, 0)
	<-done
	return nil
}
