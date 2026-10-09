//go:build windows

package display

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows Connecting and Configuring Displays (CCD) API, used to make the virtual monitor primary.

const (
	qdcAllPaths             = 0x1
	qdcOnlyActivePaths      = 0x2
	sdcTopologyExtend       = 0x4
	sdcUseSuppliedConfig    = 0x20
	sdcApply                = 0x80
	sdcAllowChanges         = 0x400
	modeInfoTypeSource      = 1
	pathModeIdxInvalid      = 0xffffffff
	errInsufficientBuffer   = 122
	primarySearchTimeout    = 5 * time.Second
	primarySearchPollPeriod = 100 * time.Millisecond
)

type luid struct {
	Low  uint32
	High int32
}

type pathSourceInfo struct {
	Adapter     luid
	ID          uint32
	ModeInfoIdx uint32
	StatusFlags uint32
}

type pathTargetInfo struct {
	Adapter          luid
	ID               uint32
	ModeInfoIdx      uint32
	OutputTechnology uint32
	Rotation         uint32
	Scaling          uint32
	RefreshNum       uint32
	RefreshDen       uint32
	ScanLineOrdering uint32
	TargetAvailable  int32
	StatusFlags      uint32
}

type pathInfo struct {
	Source pathSourceInfo
	Target pathTargetInfo
	Flags  uint32
}

// modeInfo is DISPLAYCONFIG_MODE_INFO; the union holds a source or target mode.
type modeInfo struct {
	InfoType uint32
	ID       uint32
	Adapter  luid
	Union    [48]byte
}

// sourceMode is DISPLAYCONFIG_SOURCE_MODE (the source arm of the union).
type sourceMode struct {
	Width, Height, PixelFormat uint32
	X, Y                       int32
}

func (m *modeInfo) source() *sourceMode { return (*sourceMode)(unsafe.Pointer(&m.Union[0])) }

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	procGetDisplayConfigBufferSize = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig         = user32.NewProc("QueryDisplayConfig")
	procSetDisplayConfig           = user32.NewProc("SetDisplayConfig")
)

func queryConfig(flags uint32) ([]pathInfo, []modeInfo, error) {
	for {
		var np, nm uint32
		if r, _, _ := procGetDisplayConfigBufferSize.Call(uintptr(flags), uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 {
			return nil, nil, fmt.Errorf("GetDisplayConfigBufferSizes: %w", windows.Errno(r))
		}
		paths := make([]pathInfo, np+1)
		modes := make([]modeInfo, nm+1)
		r, _, _ := procQueryDisplayConfig.Call(uintptr(flags), uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&paths[0])),
			uintptr(unsafe.Pointer(&nm)), uintptr(unsafe.Pointer(&modes[0])), 0)
		if r == errInsufficientBuffer {
			continue // a display was added in between
		}
		if r != 0 {
			return nil, nil, fmt.Errorf("QueryDisplayConfig: %w", windows.Errno(r))
		}
		return paths[:np], modes[:nm], nil
	}
}

func setConfig(paths []pathInfo, modes []modeInfo, flags uint32) error {
	var pp, mp uintptr
	if len(paths) > 0 {
		pp = uintptr(unsafe.Pointer(&paths[0]))
	}
	if len(modes) > 0 {
		mp = uintptr(unsafe.Pointer(&modes[0]))
	}
	if r, _, _ := procSetDisplayConfig.Call(uintptr(len(paths)), pp, uintptr(len(modes)), mp, uintptr(flags)); r != 0 {
		return fmt.Errorf("SetDisplayConfig: %w", windows.Errno(r))
	}
	return nil
}

// findActivePath returns the index of the active path driving the given target.
func findActivePath(paths []pathInfo, adapter luid, target uint32) int {
	for i, p := range paths {
		if p.Target.Adapter == adapter && p.Target.ID == target {
			return i
		}
	}
	return -1
}

// makePrimary makes the display on (adapter, target) the primary one by moving it to the desktop
// origin and shifting every other display by the same offset, so the arrangement is kept.
// It waits for Windows to activate the new display and extends the desktop if it doesn't.
func makePrimary(adapter luid, target uint32) error {
	deadline := time.Now().Add(primarySearchTimeout)
	extended := false
	for {
		paths, modes, err := queryConfig(qdcOnlyActivePaths)
		if err != nil {
			return err
		}
		if i := findActivePath(paths, adapter, target); i >= 0 {
			idx := paths[i].Source.ModeInfoIdx
			if idx == pathModeIdxInvalid || int(idx) >= len(modes) || modes[idx].InfoType != modeInfoTypeSource {
				return fmt.Errorf("virtual display has no source mode")
			}
			dx, dy := modes[idx].source().X, modes[idx].source().Y
			if dx == 0 && dy == 0 {
				return nil
			}
			for j := range modes {
				if modes[j].InfoType == modeInfoTypeSource {
					modes[j].source().X -= dx
					modes[j].source().Y -= dy
				}
			}
			return setConfig(paths, modes, sdcApply|sdcUseSuppliedConfig|sdcAllowChanges)
		}
		if time.Now().After(deadline) {
			if extended {
				return fmt.Errorf("virtual display did not become active")
			}
			// The display database may keep the new monitor off; extend the desktop to include it.
			if err := setConfig(nil, nil, sdcApply|sdcTopologyExtend); err != nil {
				return err
			}
			extended, deadline = true, time.Now().Add(primarySearchTimeout)
		}
		time.Sleep(primarySearchPollPeriod)
	}
}
