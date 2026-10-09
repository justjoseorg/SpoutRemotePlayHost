//go:build windows

package display

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Steam creates its own virtual display (SudoVDA) for a Remote Play session. This backend
// does not create one: it waits for that display to appear, then switches off every
// other monitor and restores the saved layout when the session ends.

const (
	qdcOnlyActivePaths = 0x2

	sdcApply                     = 0x80
	sdcUseSuppliedDisplayConfig  = 0x20
	sdcAllowChanges              = 0x400
	sdcTopologyExtend            = 0x4
	errorInsufficientBuffer      = 122
	targetDeviceName             = 2
	adapterName                  = 4
	waitForVirtual               = 20 * time.Second
	pollInterval                 = 500 * time.Millisecond
	displayConfigTargetNameSize  = 20 + 4 + 4 + 2 + 2 + 4 + 64*2 + 128*2
	displayConfigAdapterNameSize = 20 + 128*2
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetDisplayConfigBufSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig       = user32.NewProc("QueryDisplayConfig")
	procSetDisplayConfig         = user32.NewProc("SetDisplayConfig")
	procDisplayConfigGetInfo     = user32.NewProc("DisplayConfigGetDeviceInfo")
)

type deviceInfoHeader struct {
	Type      uint32
	Size      uint32
	AdapterID luid
	ID        uint32
}

type targetName struct {
	Header            deviceInfoHeader
	Flags             uint32
	OutputTechnology  uint32
	EdidManufactureID uint16
	EdidProductCodeID uint16
	ConnectorInstance uint32
	FriendlyName      [64]uint16
	DevicePath        [128]uint16
}

type adapterNameInfo struct {
	Header     deviceInfoHeader
	DevicePath [128]uint16
}

var (
	_ [displayConfigTargetNameSize]byte  = [unsafe.Sizeof(targetName{})]byte{}
	_ [displayConfigAdapterNameSize]byte = [unsafe.Sizeof(adapterNameInfo{})]byte{}
)

func newBackend() backend { return &isolator{} }

type isolator struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	saved  *savedTopology
}

type savedTopology struct {
	paths []pathInfo
	modes []modeInfo
}

// create ignores the requested mode: Steam sizes its virtual display itself. It returns
// at once; the monitors are switched off in the background once the display exists.
func (b *isolator) create(Mode) error {
	if err := b.destroy(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	b.mu.Lock()
	b.cancel, b.done = cancel, done
	b.mu.Unlock()
	go func() {
		defer close(done)
		if err := b.isolate(ctx); err != nil {
			log.Println("physical monitors left on:", err)
		}
	}()
	return nil
}

func (b *isolator) isolate(ctx context.Context) error {
	deadline := time.Now().Add(waitForVirtual)
	for {
		paths, modes, err := queryActive()
		if err != nil {
			return err
		}
		out, outModes, n := keepOnly(paths, modes, isVirtual)
		if n > 0 {
			if n == len(paths) {
				return nil // already only virtual displays
			}
			b.mu.Lock()
			if ctx.Err() != nil {
				b.mu.Unlock()
				return nil
			}
			if err := setConfig(out, outModes); err != nil {
				b.mu.Unlock()
				return fmt.Errorf("disable physical monitors: %w", err)
			}
			b.saved = &savedTopology{paths: paths, modes: modes}
			b.mu.Unlock()
			log.Printf("disabled %d physical monitor(s) for the Remote Play session", len(paths)-n)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Steam's virtual display did not appear within %s", waitForVirtual)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollInterval):
		}
	}
}

func (b *isolator) destroy() error {
	b.mu.Lock()
	cancel, done := b.cancel, b.done
	b.cancel, b.done = nil, nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.saved == nil {
		return nil
	}
	s := b.saved
	b.saved = nil
	if err := setConfig(s.paths, s.modes); err != nil {
		// The saved layout can include Steam's virtual display, which is gone by now.
		if err2 := setTopology(sdcTopologyExtend); err2 != nil {
			return fmt.Errorf("restore monitors: %w (fallback: %v)", err, err2)
		}
	}
	return nil
}

// isVirtual matches Steam's SudoVDA display by its monitor or adapter name.
func isVirtual(p pathInfo) bool {
	t, ok := targetInfo(p)
	if ok {
		if virtualName(windows.UTF16ToString(t.FriendlyName[:]) + " " + windows.UTF16ToString(t.DevicePath[:])) {
			return true
		}
	}
	if a, ok := adapterInfo(p); ok && virtualName(windows.UTF16ToString(a.DevicePath[:])) {
		return true
	}
	return false
}

func targetInfo(p pathInfo) (targetName, bool) {
	var t targetName
	t.Header = deviceInfoHeader{Type: targetDeviceName, Size: uint32(unsafe.Sizeof(t)), AdapterID: p.TgtAdapter, ID: p.TgtID}
	r, _, _ := procDisplayConfigGetInfo.Call(uintptr(unsafe.Pointer(&t)))
	return t, r == 0
}

func adapterInfo(p pathInfo) (adapterNameInfo, bool) {
	var a adapterNameInfo
	a.Header = deviceInfoHeader{Type: adapterName, Size: uint32(unsafe.Sizeof(a)), AdapterID: p.TgtAdapter}
	r, _, _ := procDisplayConfigGetInfo.Call(uintptr(unsafe.Pointer(&a)))
	return a, r == 0
}

func queryActive() ([]pathInfo, []modeInfo, error) {
	for i := 0; i < 5; i++ {
		var np, nm uint32
		if r, _, _ := procGetDisplayConfigBufSizes.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 {
			return nil, nil, fmt.Errorf("GetDisplayConfigBufferSizes: error %d", r)
		}
		paths := make([]pathInfo, np)
		modes := make([]modeInfo, nm)
		var pp, mp unsafe.Pointer
		if np > 0 {
			pp = unsafe.Pointer(&paths[0])
		}
		if nm > 0 {
			mp = unsafe.Pointer(&modes[0])
		}
		r, _, _ := procQueryDisplayConfig.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&np)), uintptr(pp),
			uintptr(unsafe.Pointer(&nm)), uintptr(mp), 0)
		if r == errorInsufficientBuffer {
			continue
		}
		if r != 0 {
			return nil, nil, fmt.Errorf("QueryDisplayConfig: error %d", r)
		}
		return paths[:np], modes[:nm], nil
	}
	return nil, nil, fmt.Errorf("QueryDisplayConfig: display configuration kept changing")
}

func setConfig(paths []pathInfo, modes []modeInfo) error {
	var pp, mp unsafe.Pointer
	if len(paths) > 0 {
		pp = unsafe.Pointer(&paths[0])
	}
	if len(modes) > 0 {
		mp = unsafe.Pointer(&modes[0])
	}
	r, _, _ := procSetDisplayConfig.Call(uintptr(len(paths)), uintptr(pp), uintptr(len(modes)), uintptr(mp),
		sdcApply|sdcUseSuppliedDisplayConfig|sdcAllowChanges)
	if r != 0 {
		return fmt.Errorf("SetDisplayConfig: error %d", r)
	}
	return nil
}

func setTopology(flag uintptr) error {
	r, _, _ := procSetDisplayConfig.Call(0, 0, 0, 0, sdcApply|flag)
	if r != 0 {
		return fmt.Errorf("SetDisplayConfig topology: error %d", r)
	}
	return nil
}
