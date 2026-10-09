//go:build windows

package display

import "testing"

func TestVirtualName(t *testing.T) {
	for s, want := range map[string]bool{
		// Steam's SudoVDA display, named after the client.
		`\\?\DISPLAY#SMKD1CE#1&28a6823a&2&UID264#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`: true,
		"SudoMaker Virtual Display Adapter": true,
		"ayn-odin-2-po":                     false,
		"Odyssey G85SB":                     false,
		`\\?\DISPLAY#SAM72F2#5&2531ebf4&0&UID4352#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`: false,
	} {
		if got := virtualName(s); got != want {
			t.Errorf("virtualName(%q) = %v, want %v", s, got, want)
		}
	}
}
