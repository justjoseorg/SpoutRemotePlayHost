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
	sdcSaveToDatabase       = 0x200
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

// waitActive waits for Windows to activate the display on (adapter, target), extending the
// desktop if the display database keeps it off, and returns the active config.
func waitActive(adapter luid, target uint32) ([]pathInfo, []modeInfo, int, error) {
	deadline := time.Now().Add(primarySearchTimeout)
	extended := false
	for {
		paths, modes, err := queryConfig(qdcOnlyActivePaths)
		if err != nil {
			return nil, nil, -1, err
		}
		if i := findActivePath(paths, adapter, target); i >= 0 {
			return paths, modes, i, nil
		}
		if time.Now().After(deadline) {
			if extended {
				return nil, nil, -1, fmt.Errorf("virtual display did not become active")
			}
			if err := setConfig(nil, nil, sdcApply|sdcTopologyExtend); err != nil {
				return nil, nil, -1, err
			}
			extended, deadline = true, time.Now().Add(primarySearchTimeout)
		}
		time.Sleep(primarySearchPollPeriod)
	}
}

func sourceOf(p pathInfo, modes []modeInfo) *sourceMode {
	idx := p.Source.ModeInfoIdx
	if idx == pathModeIdxInvalid || int(idx) >= len(modes) || modes[idx].InfoType != modeInfoTypeSource {
		return nil
	}
	return modes[idx].source()
}

// isolate makes the display on (adapter, target) the primary one at the desktop origin and
// turns off every other display except those whose ID is in keep. Kept displays are placed to
// its right, keeping their arrangement.
func isolate(adapter luid, target uint32, keep []string) error {
	paths, modes, vi, err := waitActive(adapter, target)
	if err != nil {
		return err
	}
	keepSet := map[string]bool{}
	for _, id := range keep {
		keepSet[id] = true
	}
	var np []pathInfo
	var nm []modeInfo
	remap := map[uint32]uint32{}
	addMode := func(idx uint32) uint32 {
		if idx == pathModeIdxInvalid || int(idx) >= len(modes) {
			return idx
		}
		if n, ok := remap[idx]; ok {
			return n
		}
		nm = append(nm, modes[idx])
		remap[idx] = uint32(len(nm) - 1)
		return remap[idx]
	}
	add := func(p pathInfo) {
		p.Source.ModeInfoIdx = addMode(p.Source.ModeInfoIdx)
		p.Target.ModeInfoIdx = addMode(p.Target.ModeInfoIdx)
		np = append(np, p)
	}
	add(paths[vi])
	for i, p := range paths {
		if i != vi && keepSet[targetID(p.Target.Adapter, p.Target.ID)] {
			add(p)
		}
	}
	v := sourceOf(np[0], nm)
	if v == nil {
		return fmt.Errorf("virtual display has no source mode")
	}
	// Kept displays: shift their group so its left edge touches the virtual display's right edge.
	minX, minY, first := int32(0), int32(0), true
	for _, p := range np[1:] {
		if s := sourceOf(p, nm); s != nil && s != v {
			if first || s.X < minX {
				minX = s.X
			}
			if first || s.Y < minY {
				minY = s.Y
			}
			first = false
		}
	}
	for _, p := range np[1:] {
		if s := sourceOf(p, nm); s != nil && s != v {
			s.X += int32(v.Width) - minX
			s.Y -= minY
		}
	}
	v.X, v.Y = 0, 0
	// Saved so Windows re-applies it itself when a display reconnects (an off monitor that
	// sleeps can drop off the bus and come back). The database entry belongs to the set of
	// connected displays including the virtual one, so the normal layout is untouched.
	if err := setConfig(np, nm, sdcApply|sdcUseSuppliedConfig|sdcAllowChanges|sdcSaveToDatabase); err == nil {
		return nil
	}
	return setConfig(np, nm, sdcApply|sdcUseSuppliedConfig|sdcAllowChanges)
}

// drifted reports whether the active layout no longer matches isolate's: the display on
// (adapter, target) is not active or not at the origin, or another display not in keep is on.
func drifted(adapter luid, target uint32, keep []string) bool {
	paths, modes, err := queryConfig(qdcOnlyActivePaths)
	if err != nil {
		return false
	}
	vi := findActivePath(paths, adapter, target)
	if vi < 0 {
		return true
	}
	if s := sourceOf(paths[vi], modes); s == nil || s.X != 0 || s.Y != 0 {
		return true
	}
	keepSet := map[string]bool{}
	for _, id := range keep {
		keepSet[id] = true
	}
	for i, p := range paths {
		if i != vi && !keepSet[targetID(p.Target.Adapter, p.Target.ID)] {
			return true
		}
	}
	return false
}

// restore re-applies a config saved with queryConfig, falling back to the display database.
func restore(paths []pathInfo, modes []modeInfo) error {
	err := setConfig(paths, modes, sdcApply|sdcUseSuppliedConfig|sdcAllowChanges)
	if err == nil {
		return nil
	}
	if err2 := setConfig(nil, nil, sdcApply|sdcUseDatabaseCurrent); err2 != nil {
		return fmt.Errorf("%v; database: %v", err, err2)
	}
	return nil
}

const (
	deviceInfoGetSourceName = 1
	deviceInfoGetTargetName = 2
	sdcUseDatabaseCurrent   = 0xf
)

var procDisplayConfigGetDeviceInfo = user32.NewProc("DisplayConfigGetDeviceInfo")

type deviceInfoHeader struct {
	Type, Size uint32
	Adapter    luid
	ID         uint32
}

type targetDeviceName struct {
	Header                        deviceInfoHeader
	Flags, OutputTechnology       uint32
	EdidManufacturer, EdidProduct uint16
	ConnectorInstance             uint32
	FriendlyName                  [64]uint16
	DevicePath                    [128]uint16
}

type sourceDeviceName struct {
	Header  deviceInfoHeader
	GdiName [32]uint16
}

func targetName(adapter luid, id uint32) (friendly, path string) {
	n := targetDeviceName{Header: deviceInfoHeader{Type: deviceInfoGetTargetName, Size: uint32(unsafe.Sizeof(targetDeviceName{})), Adapter: adapter, ID: id}}
	if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&n))); r != 0 {
		return "", ""
	}
	return windows.UTF16ToString(n.FriendlyName[:]), windows.UTF16ToString(n.DevicePath[:])
}

// targetID is a display's stable ID: its monitor device path (model and connector).
func targetID(adapter luid, id uint32) string {
	_, p := targetName(adapter, id)
	return p
}

func gdiName(adapter luid, id uint32) string {
	n := sourceDeviceName{Header: deviceInfoHeader{Type: deviceInfoGetSourceName, Size: uint32(unsafe.Sizeof(sourceDeviceName{})), Adapter: adapter, ID: id}}
	if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&n))); r != 0 {
		return ""
	}
	return windows.UTF16ToString(n.GdiName[:])
}

// Outputs lists the active displays, or the ones that were active before the current session
// turned them off, excluding the session's virtual display.
func Outputs() ([]Output, error) {
	paths, modes, skip := sessionLayout()
	if paths == nil {
		var err error
		if paths, modes, err = queryConfig(qdcOnlyActivePaths); err != nil {
			return nil, err
		}
	}
	var out []Output
	for _, p := range paths {
		if skip != nil && p.Target.Adapter == skip.Adapter && p.Target.ID == skip.ID {
			continue
		}
		friendly, id := targetName(p.Target.Adapter, p.Target.ID)
		if id == "" {
			continue
		}
		o := Output{ID: id, Name: friendly, Device: gdiName(p.Source.Adapter, p.Source.ID)}
		if s := sourceOf(p, modes); s != nil {
			o.Width, o.Height, o.Primary = int(s.Width), int(s.Height), s.X == 0 && s.Y == 0
		}
		if o.Name == "" {
			o.Name = o.Device
		}
		out = append(out, o)
	}
	return out, nil
}
