package display

import (
	"strings"
	"unsafe"
)

// Mirrors of the Win32 DISPLAYCONFIG_* structures. They live in a portable file so the
// topology filtering can be unit tested on any OS.

type luid struct {
	Low  uint32
	High int32
}

// pathInfo is DISPLAYCONFIG_PATH_INFO (72 bytes).
type pathInfo struct {
	SrcAdapter      luid
	SrcID           uint32
	SrcModeIdx      uint32
	SrcStatus       uint32
	TgtAdapter      luid
	TgtID           uint32
	TgtModeIdx      uint32
	Tech            uint32
	Rotation        uint32
	Scaling         uint32
	RefreshNum      uint32
	RefreshDen      uint32
	ScanLine        uint32
	TargetAvailable int32
	TgtStatus       uint32
	Flags           uint32
}

// modeInfo is DISPLAYCONFIG_MODE_INFO (64 bytes).
type modeInfo struct {
	InfoType uint32
	ID       uint32
	Adapter  luid
	Data     [48]byte
}

var (
	_ [72]byte = [unsafe.Sizeof(pathInfo{})]byte{}
	_ [64]byte = [unsafe.Sizeof(modeInfo{})]byte{}
)

const invalidModeIdx = 0xFFFFFFFF

// keepOnly returns a copy of the topology containing just the paths for which keep is
// true, with mode indexes remapped. The second result is the number of paths kept.
func keepOnly(paths []pathInfo, modes []modeInfo, keep func(pathInfo) bool) ([]pathInfo, []modeInfo, int) {
	var outPaths []pathInfo
	var outModes []modeInfo
	remap := map[uint32]uint32{}
	move := func(idx uint32) uint32 {
		if idx == invalidModeIdx || int(idx) >= len(modes) {
			return invalidModeIdx
		}
		if n, ok := remap[idx]; ok {
			return n
		}
		n := uint32(len(outModes))
		outModes = append(outModes, modes[idx])
		remap[idx] = n
		return n
	}
	for _, p := range paths {
		if !keep(p) {
			continue
		}
		p.SrcModeIdx = move(p.SrcModeIdx)
		p.TgtModeIdx = move(p.TgtModeIdx)
		outPaths = append(outPaths, p)
	}
	return outPaths, outModes, len(outPaths)
}

// virtualName reports whether a monitor/adapter description looks like the virtual
// display driver Steam uses (SudoVDA).
func virtualName(s string) bool {
	s = strings.ToLower(s)
	for _, k := range []string{"sudovda", "sudomaker", "virtual display"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
