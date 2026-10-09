package display

import "testing"

func TestKeepOnlyRemapsModes(t *testing.T) {
	paths := []pathInfo{
		{TgtID: 1, SrcModeIdx: 0, TgtModeIdx: 1},
		{TgtID: 2, SrcModeIdx: 2, TgtModeIdx: 3},
		{TgtID: 3, SrcModeIdx: 4, TgtModeIdx: invalidModeIdx},
	}
	modes := make([]modeInfo, 5)
	for i := range modes {
		modes[i].ID = uint32(100 + i)
	}
	out, outModes, n := keepOnly(paths, modes, func(p pathInfo) bool { return p.TgtID != 1 })
	if n != 2 || len(out) != 2 || len(outModes) != 3 {
		t.Fatalf("n=%d paths=%d modes=%d", n, len(out), len(outModes))
	}
	if out[0].SrcModeIdx != 0 || out[0].TgtModeIdx != 1 || out[1].SrcModeIdx != 2 || out[1].TgtModeIdx != invalidModeIdx {
		t.Fatalf("%+v", out)
	}
	if outModes[0].ID != 102 || outModes[1].ID != 103 || outModes[2].ID != 104 {
		t.Fatalf("%+v", outModes)
	}
}

func TestKeepOnlyNoneKept(t *testing.T) {
	_, _, n := keepOnly([]pathInfo{{TgtID: 1}}, nil, func(pathInfo) bool { return false })
	if n != 0 {
		t.Fatal(n)
	}
}

func TestVirtualName(t *testing.T) {
	for _, s := range []string{"SudoMaker Virtual Display", `root#sudovda`} {
		if !virtualName(s) {
			t.Errorf("%q should match", s)
		}
	}
	for _, s := range []string{"DELL U2723QE", `\\?\DISPLAY#GSM5BBF#4&1`, ""} {
		if virtualName(s) {
			t.Errorf("%q should not match", s)
		}
	}
}
