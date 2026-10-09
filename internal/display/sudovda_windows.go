//go:build windows

package display

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protocol of the SudoVDA IddCx virtual display driver (hardware id root\sudomaker\sudovda).
const (
	ioctlAddDisplay    = 0x220000 | 0x800<<2
	ioctlRemoveDisplay = 0x220000 | 0x801<<2
	ioctlGetWatchdog   = 0x220000 | 0x803<<2
	ioctlPing          = 0x220000 | 0x888<<2
	ioctlProtocol      = 0x220000 | 0x8FF<<2

	protoMajor = 0
	protoMinor = 2
)

var interfaceGUID = windows.GUID{Data1: 0xe5bcc234, Data2: 0x1e0c, Data3: 0x418a,
	Data4: [8]byte{0xa0, 0xd4, 0xef, 0x8b, 0x75, 0x01, 0x41, 0x4d}}

type addParams struct {
	Width, Height, RefreshRate uint32
	MonitorGUID                windows.GUID
	DeviceName                 [14]byte
	SerialNumber               [14]byte
}

type addOut struct {
	AdapterLUID [8]byte
	TargetID    uint32
}

type watchdogOut struct{ Timeout, Countdown uint32 }

type protocolVersion struct {
	Major, Minor, Incremental uint8
	TestBuild                 uint8
}

type sudovda struct {
	mu     sync.Mutex
	handle windows.Handle
	guid   windows.GUID
	stop   chan struct{}
}

func newBackend() backend { return &sudovda{} }

type targetRef struct {
	Adapter luid
	ID      uint32
}

// The layout from before the current session, and the session's virtual display.
var layout struct {
	sync.Mutex
	paths []pathInfo
	modes []modeInfo
	virt  *targetRef
}

func setSessionLayout(paths []pathInfo, modes []modeInfo, virt *targetRef) {
	layout.Lock()
	defer layout.Unlock()
	layout.paths, layout.modes, layout.virt = paths, modes, virt
}

func sessionLayout() ([]pathInfo, []modeInfo, *targetRef) {
	layout.Lock()
	defer layout.Unlock()
	return append([]pathInfo(nil), layout.paths...), append([]modeInfo(nil), layout.modes...), layout.virt
}

var (
	setupapi                     = windows.NewLazySystemDLL("setupapi.dll")
	procGetClassDevs             = setupapi.NewProc("SetupDiGetClassDevsW")
	procEnumDeviceInterfaces     = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procGetDeviceInterfaceDetail = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procDestroyDeviceInfoList    = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	digcfPresent         = 0x2
	digcfDeviceInterface = 0x10
)

type deviceInterfaceData struct {
	Size     uint32
	Class    windows.GUID
	Flags    uint32
	Reserved uintptr
}

func openDevice() (windows.Handle, error) {
	set, _, err := procGetClassDevs.Call(uintptr(unsafe.Pointer(&interfaceGUID)), 0, 0, digcfPresent|digcfDeviceInterface)
	if windows.Handle(set) == windows.InvalidHandle {
		return 0, fmt.Errorf("SetupDiGetClassDevs: %w", err)
	}
	defer procDestroyDeviceInfoList.Call(set)

	var data deviceInterfaceData
	data.Size = uint32(unsafe.Sizeof(data))
	if r, _, _ := procEnumDeviceInterfaces.Call(set, 0, uintptr(unsafe.Pointer(&interfaceGUID)), 0, uintptr(unsafe.Pointer(&data))); r == 0 {
		return 0, fmt.Errorf("SudoVDA driver not found; install it first")
	}

	var need uint32
	procGetDeviceInterfaceDetail.Call(set, uintptr(unsafe.Pointer(&data)), 0, 0, uintptr(unsafe.Pointer(&need)), 0)
	if need == 0 {
		return 0, fmt.Errorf("SetupDiGetDeviceInterfaceDetail: no size")
	}
	buf := make([]byte, need)
	// cbSize of SP_DEVICE_INTERFACE_DETAIL_DATA_W is 8 on 64-bit, 6 on 32-bit.
	cb := uint32(6)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		cb = 8
	}
	*(*uint32)(unsafe.Pointer(&buf[0])) = cb
	if r, _, err := procGetDeviceInterfaceDetail.Call(set, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&buf[0])), uintptr(need), 0, 0); r == 0 {
		return 0, fmt.Errorf("SetupDiGetDeviceInterfaceDetail: %w", err)
	}
	path := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(&buf[4])))
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
}

func ioctl(h windows.Handle, code uint32, in, out unsafe.Pointer, inSize, outSize uint32) error {
	var returned uint32
	return windows.DeviceIoControl(h, code, (*byte)(in), inSize, (*byte)(out), outSize, &returned, nil)
}

func (s *sudovda) create(m Mode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, err := openDevice()
	if err != nil {
		return err
	}
	var ver protocolVersion
	if err := ioctl(h, ioctlProtocol, nil, unsafe.Pointer(&ver), 0, uint32(unsafe.Sizeof(ver))); err != nil {
		windows.CloseHandle(h)
		return fmt.Errorf("read driver protocol: %w", err)
	}
	if ver.Major != protoMajor || ver.Minor < protoMinor {
		windows.CloseHandle(h)
		return fmt.Errorf("unsupported SudoVDA protocol %d.%d.%d", ver.Major, ver.Minor, ver.Incremental)
	}

	guid, err := windows.GenerateGUID()
	if err != nil {
		windows.CloseHandle(h)
		return err
	}
	// Remember the layout before the virtual display exists so it can be restored exactly.
	origPaths, origModes, err := queryConfig(qdcOnlyActivePaths)
	if err != nil {
		log.Println("could not save the display layout:", err)
	}
	p := addParams{Width: uint32(m.Width), Height: uint32(m.Height), RefreshRate: uint32(m.RefreshHz), MonitorGUID: guid}
	copy(p.DeviceName[:13], "SpoutRemote")
	copy(p.SerialNumber[:13], "SPOUT0001")
	var out addOut
	if err := ioctl(h, ioctlAddDisplay, unsafe.Pointer(&p), unsafe.Pointer(&out), uint32(unsafe.Sizeof(p)), uint32(unsafe.Sizeof(out))); err != nil {
		windows.CloseHandle(h)
		return fmt.Errorf("add virtual display: %w", err)
	}

	s.handle, s.guid, s.stop = h, guid, make(chan struct{})
	go s.keepAlive(h, s.stop)

	// Steam streams the primary display, so the virtual display becomes the only one (plus the
	// displays the user chose to keep). Nothing is saved; destroy puts the old layout back.
	adapter := luid{Low: binary.LittleEndian.Uint32(out.AdapterLUID[:4]), High: int32(binary.LittleEndian.Uint32(out.AdapterLUID[4:]))}
	setSessionLayout(origPaths, origModes, &targetRef{adapter, out.TargetID})
	if err := isolate(adapter, out.TargetID, keepOutputs()); err != nil {
		log.Println("virtual display created but other displays not turned off:", err)
	}
	return nil
}

// The driver removes displays when it stops being pinged, so keep it alive.
func (s *sudovda) keepAlive(h windows.Handle, stop chan struct{}) {
	interval := time.Second
	var wd watchdogOut
	if err := ioctl(h, ioctlGetWatchdog, nil, unsafe.Pointer(&wd), 0, uint32(unsafe.Sizeof(wd))); err == nil && wd.Timeout > 1 {
		interval = time.Duration(wd.Timeout) * time.Second / 3
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = ioctl(h, ioctlPing, nil, nil, 0, 0)
		}
	}
}

func (s *sudovda) destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return nil
	}
	close(s.stop)
	// Bring the physical displays back before removing the virtual one, so there is never a
	// moment without any active display.
	if paths, modes, _ := sessionLayout(); paths != nil {
		if err := restore(paths, modes); err != nil {
			log.Println("could not restore the display layout:", err)
		}
	}
	setSessionLayout(nil, nil, nil)
	err := ioctl(s.handle, ioctlRemoveDisplay, unsafe.Pointer(&s.guid), nil, uint32(unsafe.Sizeof(s.guid)), 0)
	windows.CloseHandle(s.handle)
	s.handle = 0
	if err != nil {
		return fmt.Errorf("remove virtual display: %w", err)
	}
	return nil
}
