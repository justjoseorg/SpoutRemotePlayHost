//go:build windows

package signin

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
)

// ServiceName is the Windows service the installer registers.
const ServiceName = "SpoutSignIn"

// Exit codes of the typing helper.
const (
	exitOK        = 0
	exitBadInput  = 2
	exitNotSignIn = 3
	exitDesktop   = 4
	exitSendInput = 5

	vkBack   = 0x08
	vkReturn = 0x0D
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procOpenInputDesktop    = user32.NewProc("OpenInputDesktop")
	procCloseDesktop        = user32.NewProc("CloseDesktop")
	procSetThreadDesktop    = user32.NewProc("SetThreadDesktop")
	procGetUserObjectInfo   = user32.NewProc("GetUserObjectInformationW")
	procSendInput           = user32.NewProc("SendInput")
	procMapVirtualKey       = user32.NewProc("MapVirtualKeyW")
	wtsapi32                = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInfo = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory       = wtsapi32.NewProc("WTSFreeMemory")
)

// RunService serves the sign-in API. Paired devices are read from pairedPath on every request,
// so devices paired after the service started work without a restart.
func RunService(listen, pairedPath string) error {
	valid := func(token string) bool {
		m, err := pairing.New(pairedPath, nil)
		return err == nil && m.Valid(token)
	}
	h := NewHandler(valid, launchTyper)
	h.AtSignIn = func() bool {
		session := windows.WTSGetActiveConsoleSessionId()
		return session != 0xFFFFFFFF && signedOutOrLocked(session)
	}
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isSvc {
		ln, err := net.Listen("tcp", listen)
		if err != nil {
			return err
		}
		return newServer(h).Serve(ln)
	}
	return svc.Run(ServiceName, &service{listen: listen, h: h})
}

func newServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second}
}

type service struct {
	listen string
	h      http.Handler
}

func (s *service) Execute(_ []string, req <-chan svc.ChangeRequest, st chan<- svc.Status) (bool, uint32) {
	st <- svc.Status{State: svc.StartPending}
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		log.Println("sign-in service: listen:", err)
		return false, 1
	}
	srv := newServer(s.h)
	go func() { _ = srv.Serve(ln) }()
	log.Println("sign-in service listening on", s.listen)
	st <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range req {
		switch c.Cmd {
		case svc.Interrogate:
			st <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			st <- svc.Status{State: svc.StopPending}
			_ = srv.Close()
			return false, 0
		}
	}
	return false, 0
}

// launchTyper starts this exe with -signin-type in the console session, on the sign-in desktop,
// and hands it the PIN through a pipe so it never appears on a command line.
func launchTyper(pin string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	session := windows.WTSGetActiveConsoleSessionId()
	if session == 0xFFFFFFFF {
		return errors.New("no console session")
	}
	// UAC prompts also use the Winlogon desktop, so only type when nobody is signed in or the session is locked.
	if !signedOutOrLocked(session) {
		return ErrNotSignInScreen
	}
	var own windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ADJUST_SESSIONID, &own); err != nil {
		return fmt.Errorf("open token: %w", err)
	}
	defer own.Close()
	var tok windows.Token
	if err := windows.DuplicateTokenEx(own, windows.MAXIMUM_ALLOWED, nil, windows.SecurityIdentification, windows.TokenPrimary, &tok); err != nil {
		return fmt.Errorf("duplicate token: %w", err)
	}
	defer tok.Close()
	if err := windows.SetTokenInformation(tok, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4); err != nil {
		return fmt.Errorf("set session: %w", err)
	}

	var rd, wr windows.Handle
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	if err := windows.CreatePipe(&rd, &wr, &sa, 0); err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	defer windows.CloseHandle(rd)
	defer func() {
		if wr != windows.InvalidHandle {
			_ = windows.CloseHandle(wr)
		}
	}()
	if err := windows.SetHandleInformation(wr, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return err
	}

	si := windows.StartupInfo{
		Desktop:    windows.StringToUTF16Ptr(`winsta0\Winlogon`),
		Flags:      windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW,
		ShowWindow: windows.SW_HIDE,
		StdInput:   rd,
	}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	cmd := windows.StringToUTF16Ptr(syscall.EscapeArg(exe) + " -signin-type")
	if err := windows.CreateProcessAsUser(tok, nil, cmd, nil, nil, true, windows.CREATE_NO_WINDOW, nil, nil, &si, &pi); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)

	var n uint32
	_ = windows.WriteFile(wr, []byte(pin), &n, nil)
	_ = windows.CloseHandle(wr)
	wr = windows.InvalidHandle

	ev, err := windows.WaitForSingleObject(pi.Process, 30_000)
	if err != nil || ev != windows.WAIT_OBJECT_0 {
		_ = windows.TerminateProcess(pi.Process, 1)
		return errors.New("typing the PIN timed out")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(pi.Process, &code); err != nil {
		return err
	}
	switch code {
	case exitOK:
		log.Println("sign-in: PIN typed")
		return nil
	case exitNotSignIn:
		return ErrNotSignInScreen
	default:
		return fmt.Errorf("typing the PIN failed (helper exit %d)", code)
	}
}

// TypeFromStdin is the helper: it reads the PIN from stdin and types it into the sign-in screen.
// It refuses when the input desktop is not the sign-in desktop, so it never types into a session.
func TypeFromStdin() int {
	runtime.LockOSThread()
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 64))
	if err != nil {
		return exitBadInput
	}
	pin := strings.TrimSpace(string(raw))
	if !pinRE.MatchString(pin) {
		return exitBadInput
	}
	// SendInput needs DESKTOP_JOURNALPLAYBACK on the thread's desktop, which GENERIC_ALL includes.
	const genericAll = 0x10000000
	d, _, _ := procOpenInputDesktop.Call(0, 0, genericAll)
	if d == 0 {
		return exitDesktop
	}
	defer procCloseDesktop.Call(d)
	if !strings.EqualFold(desktopName(d), "Winlogon") {
		return exitNotSignIn
	}
	if r, _, _ := procSetThreadDesktop.Call(d); r == 0 {
		return exitDesktop
	}

	// Enter lifts the lock screen picture (Space and Ctrl didn't on a tested PC).
	// Clear the PIN box first so that Enter can't submit a half-typed PIN, then
	// again in case the box was already showing.
	if !clearBox() || !tap(vkReturn) {
		return exitSendInput
	}
	time.Sleep(2500 * time.Millisecond)
	if !clearBox() {
		return exitSendInput
	}
	for _, c := range pin {
		if !tap(uint16(c)) { // VK codes for 0-9 are their ASCII digits
			return exitSendInput
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !tap(vkReturn) {
		return exitSendInput
	}
	// If the lock screen picture took the first Enter, the PIN is still waiting
	// in the box: press Enter once more while the sign-in screen has the keyboard.
	time.Sleep(2 * time.Second)
	if stillAtSignIn() && !tap(vkReturn) {
		return exitSendInput
	}
	return exitOK
}

func stillAtSignIn() bool {
	const desktopReadObjects = 0x0001
	d, _, _ := procOpenInputDesktop.Call(0, 0, desktopReadObjects)
	if d == 0 {
		return false
	}
	defer procCloseDesktop.Call(d)
	return strings.EqualFold(desktopName(d), "Winlogon")
}

func clearBox() bool {
	for i := 0; i < 34; i++ {
		if !tap(vkBack) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func wtsQuery(session, class uint32) []byte {
	var buf *byte
	var n uint32
	r, _, _ := procWTSQuerySessionInfo.Call(0, uintptr(session), uintptr(class), uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || buf == nil {
		return nil
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buf)))
	return append([]byte(nil), unsafe.Slice(buf, n)...)
}

// signedOutOrLocked reports whether the console session has no user, or LogonUI
// (the lock and sign-in screen) is showing in it. The WTS lock flag isn't used:
// its values are reversed on some Windows builds.
func signedOutOrLocked(session uint32) bool {
	const wtsUserName = 5
	name := wtsQuery(session, wtsUserName)
	if len(name) < 2 || name[0] == 0 && name[1] == 0 {
		return true
	}
	return processInSession("logonui.exe", session)
}

func processInSession(exe string, session uint32) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			continue
		}
		var s uint32
		if windows.ProcessIdToSessionId(e.ProcessID, &s) == nil && s == session {
			return true
		}
	}
	return false
}

func desktopName(d uintptr) string {
	buf := make([]uint16, 64)
	var need uint32
	const uoiName = 2
	r, _, _ := procGetUserObjectInfo.Call(d, uoiName, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&need)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// keyInput mirrors INPUT with a KEYBDINPUT on 64-bit Windows (40 bytes).
type keyInput struct {
	typ   uint32
	_     uint32
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	extra uint64
	_     [8]byte
}

func tap(vk uint16) bool {
	const inputKeyboard, keyUp = 1, 0x0002
	scan, _, _ := procMapVirtualKey.Call(uintptr(vk), 0)
	in := [2]keyInput{
		{typ: inputKeyboard, vk: vk, scan: uint16(scan)},
		{typ: inputKeyboard, vk: vk, scan: uint16(scan), flags: keyUp},
	}
	n, _, _ := procSendInput.Call(2, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	return n == 2
}
