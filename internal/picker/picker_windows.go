//go:build windows

package picker

import (
	"context"
	"fmt"
	"runtime"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32                 = windows.NewLazySystemDLL("comdlg32.dll")
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	procGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	procEnumThreadWindows    = user32.NewProc("EnumThreadWindows")
	procPostMessageW         = user32.NewProc("PostMessageW")
)

// openFileName is OPENFILENAMEW.
type openFileName struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	Reserved      uintptr
	Reserved2     uint32
	FlagsEx       uint32
}

const (
	ofnHideReadOnly  = 0x4
	ofnNoChangeDir   = 0x8
	ofnPathMustExist = 0x800
	ofnFileMustExist = 0x1000
	ofnExplorer      = 0x80000

	wmCommand = 0x0111
	idCancel  = 2
)

// cancelWindow presses Cancel on a window of the dialog's thread.
var cancelWindow = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
	procPostMessageW.Call(hwnd, wmCommand, idCancel, 0)
	return 1
})

// utf16z converts s (which may contain NULs, as filter strings do) to a NUL-terminated UTF-16 buffer.
func utf16z(s string) []uint16 {
	return append(utf16.Encode([]rune(s)), 0)
}

func openFile(ctx context.Context, dir string) (string, error) {
	type result struct {
		path string
		err  error
	}
	ch := make(chan result, 1)
	tid := make(chan uint32, 1)
	go func() {
		// The dialog is a COM STA client and must stay on one OS thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid <- windows.GetCurrentThreadId()
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
			defer windows.CoUninitialize()
		}
		p, err := showDialog(dir)
		ch <- result{p, err}
	}()
	thread := <-tid
	select {
	case r := <-ch:
		return r.path, r.err
	case <-ctx.Done():
	}
	// Nobody waits for the answer any more (e.g. the browser tab closed): close the dialog so it
	// doesn't linger and block the next Browse. Retry in case it was still being created.
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		procEnumThreadWindows.Call(uintptr(thread), cancelWindow, 0)
		select {
		case <-ch:
			return "", ctx.Err()
		case <-tick.C:
		}
	}
}

func showDialog(dir string) (string, error) {
	file := make([]uint16, 32768)
	filter := utf16z("Programs (*.exe, *.bat, *.cmd, *.lnk)\x00*.exe;*.bat;*.cmd;*.lnk\x00All files\x00*.*\x00")
	title := utf16z("Choose a program for Steam")
	ofn := openFileName{
		Filter:      &filter[0],
		FilterIndex: 1,
		File:        &file[0],
		MaxFile:     uint32(len(file)),
		Title:       &title[0],
		Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir,
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	// Owning the dialog with the browser window the user just clicked in keeps it in front.
	ofn.Owner, _, _ = procGetForegroundWindow.Call()
	if dir != "" {
		d := utf16z(dir)
		ofn.InitialDir = &d[0]
	}
	ok, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ok == 0 {
		if code, _, _ := procCommDlgExtendedError.Call(); code != 0 {
			return "", fmt.Errorf("file dialog failed (error 0x%x)", code)
		}
		return "", nil
	}
	return windows.UTF16ToString(file), nil
}
