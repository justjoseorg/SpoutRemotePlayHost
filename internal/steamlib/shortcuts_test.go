package steamlib

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

type vdfBuf struct{ bytes.Buffer }

func (b *vdfBuf) key(t byte, k string) { b.WriteByte(t); b.WriteString(k); b.WriteByte(0) }
func (b *vdfBuf) str(k, v string)      { b.key(vdfString, k); b.WriteString(v); b.WriteByte(0) }
func (b *vdfBuf) u32(k string, v uint32) {
	b.key(vdfInt32, k)
	_ = binary.Write(b, binary.LittleEndian, v)
}

func shortcutsBlob() []byte {
	var b vdfBuf
	b.key(vdfMap, "shortcuts")
	b.key(vdfMap, "0")
	b.u32("appid", 3123456789)
	b.str("AppName", "Celeste")
	b.str("Exe", `"C:\Games\Celeste\Celeste.exe"`)
	b.str("StartDir", `"C:\Games\Celeste\"`)
	b.str("LaunchOptions", "-windowed")
	b.u32("IsHidden", 0)
	b.key(vdfUint64, "LastPlayTime")
	b.Write(make([]byte, 8))
	b.key(vdfMap, "tags")
	b.str("0", "favorite")
	b.WriteByte(vdfEnd)
	b.WriteByte(vdfEnd)
	b.key(vdfMap, "1")
	b.str("appname", "Old")
	b.str("exe", "/usr/bin/old")
	b.WriteByte(vdfEnd)
	b.WriteByte(vdfEnd)
	b.WriteByte(vdfEnd)
	return b.Bytes()
}

func TestParseShortcuts(t *testing.T) {
	got, err := ParseShortcuts(shortcutsBlob())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %+v", got)
	}
	want := Existing{AppID: 3123456789, Name: "Celeste", Exe: `C:\Games\Celeste\Celeste.exe`, StartDir: `C:\Games\Celeste\`, Args: "-windowed"}
	if got[0] != want {
		t.Fatalf("got %+v", got[0])
	}
	if id := crc32.ChecksumIEEE([]byte("/usr/bin/oldOld")) | 0x80000000; got[1].AppID != id || got[1].Name != "Old" {
		t.Fatalf("legacy entry: %+v", got[1])
	}
}

func TestParseShortcutsTruncated(t *testing.T) {
	blob := shortcutsBlob()
	for _, n := range []int{5, 20, 60, len(blob) - 4} {
		if _, err := ParseShortcuts(blob[:n]); err == nil {
			t.Errorf("cut at %d: want error", n)
		}
	}
	if got, err := ParseShortcuts(nil); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
}

func TestShortcutsFileUsesMostRecentUser(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"111", "222"} {
		dir := filepath.Join(root, "userdata", id, "config")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "shortcuts.vdf"), []byte{8}, 0o644)
	}
	_ = os.MkdirAll(filepath.Join(root, "config"), 0o755)
	login := "\"users\"\n{\n\t\"76561197960265950\"\n\t{\n\t\t\"AccountName\"\t\t\"a\"\n\t\t\"MostRecent\"\t\t\"1\"\n\t}\n}\n"
	_ = os.WriteFile(filepath.Join(root, "config", "loginusers.vdf"), []byte(login), 0o644)
	p, err := shortcutsFile([]string{filepath.Join(root, "missing"), root})
	if err != nil || p != filepath.Join(root, "userdata", "222", "config", "shortcuts.vdf") {
		t.Fatalf("got %q %v", p, err)
	}
}
