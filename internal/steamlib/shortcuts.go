package steamlib

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Existing is a non-Steam shortcut already in the user's Steam library.
type Existing struct {
	AppID    uint32 `json:"appId"`
	Name     string `json:"name"`
	Exe      string `json:"exe"`
	StartDir string `json:"startDir,omitempty"`
	Args     string `json:"args,omitempty"`
}

// steamID64Base converts a SteamID64 to the account id used for userdata folder names.
const steamID64Base = 76561197960265728

// LocalShortcuts reads the non-Steam shortcuts of the Steam user who signed in most recently.
func LocalShortcuts() ([]Existing, error) {
	path, err := shortcutsFile(Roots())
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseShortcuts(data)
}

// shortcutsFile finds userdata/<account>/config/shortcuts.vdf for the most recent Steam user,
// falling back to the most recently changed one.
func shortcutsFile(roots []string) (string, error) {
	for _, root := range roots {
		users, _ := filepath.Glob(filepath.Join(root, "userdata", "*", "config"))
		if len(users) == 0 {
			continue
		}
		if id := mostRecentAccount(filepath.Join(root, "config", "loginusers.vdf")); id != "" {
			p := filepath.Join(root, "userdata", id, "config", "shortcuts.vdf")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		var best string
		var bestTime int64
		for _, u := range users {
			if st, err := os.Stat(filepath.Join(u, "shortcuts.vdf")); err == nil && st.ModTime().UnixNano() > bestTime {
				best, bestTime = filepath.Join(u, "shortcuts.vdf"), st.ModTime().UnixNano()
			}
		}
		if best != "" {
			return best, nil
		}
		return filepath.Join(users[0], "shortcuts.vdf"), nil
	}
	return "", errors.New("no Steam user found on this PC")
}

var vdfPair = regexp.MustCompile(`^\s*"([^"]*)"(?:\s+"([^"]*)")?\s*$`)

// mostRecentAccount returns the userdata folder name of the user marked MostRecent in loginusers.vdf.
func mostRecentAccount(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var current string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := vdfPair.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		if m[2] == "" && len(m[1]) == 17 {
			current = m[1]
		} else if strings.EqualFold(m[1], "MostRecent") && m[2] == "1" && current != "" {
			id, err := strconv.ParseUint(current, 10, 64)
			if err != nil || id < steamID64Base {
				return ""
			}
			return strconv.FormatUint(id-steamID64Base, 10)
		}
	}
	return ""
}

// Binary VDF value types.
const (
	vdfMap    = 0x00
	vdfString = 0x01
	vdfInt32  = 0x02
	vdfFloat  = 0x03
	vdfPtr    = 0x04
	vdfColor  = 0x06
	vdfUint64 = 0x07
	vdfEnd    = 0x08
	vdfInt64  = 0x0a
)

// ParseShortcuts decodes shortcuts.vdf (Steam's binary KeyValues).
func ParseShortcuts(data []byte) ([]Existing, error) {
	r := bytes.NewReader(data)
	root, err := readVDFMap(r, 0)
	if err != nil {
		return nil, fmt.Errorf("shortcuts.vdf: %w", err)
	}
	var list map[string]any
	for k, v := range root {
		if strings.EqualFold(k, "shortcuts") {
			list, _ = v.(map[string]any)
		}
	}
	keys := make([]string, 0, len(list))
	for k := range list {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, ea := strconv.Atoi(keys[i])
		b, eb := strconv.Atoi(keys[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})
	var out []Existing
	for _, k := range keys {
		m, ok := list[k].(map[string]any)
		if !ok {
			continue
		}
		e := Existing{
			Name:     vdfStr(m, "AppName"),
			Exe:      unquote(vdfStr(m, "Exe")),
			StartDir: unquote(vdfStr(m, "StartDir")),
			Args:     vdfStr(m, "LaunchOptions"),
		}
		if e.Name == "" || e.Exe == "" {
			continue
		}
		if id, ok := vdfGet(m, "appid").(uint32); ok && id != 0 {
			e.AppID = id
		} else {
			// Entries written before Steam stored appid use this derived id.
			e.AppID = crc32.ChecksumIEEE([]byte(vdfStr(m, "Exe")+e.Name)) | 0x80000000
		}
		out = append(out, e)
	}
	return out, nil
}

func vdfGet(m map[string]any, key string) any {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil
}

func vdfStr(m map[string]any, key string) string {
	s, _ := vdfGet(m, key).(string)
	return s
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func readCString(r *bytes.Reader) (string, error) {
	var b []byte
	for {
		c, err := r.ReadByte()
		if err != nil {
			return "", io.ErrUnexpectedEOF
		}
		if c == 0 {
			return string(b), nil
		}
		b = append(b, c)
	}
}

func readVDFMap(r *bytes.Reader, depth int) (map[string]any, error) {
	if depth > 16 {
		return nil, errors.New("nested too deeply")
	}
	m := map[string]any{}
	for {
		t, err := r.ReadByte()
		if err != nil {
			if depth == 0 {
				return m, nil
			}
			return nil, io.ErrUnexpectedEOF
		}
		if t == vdfEnd {
			return m, nil
		}
		key, err := readCString(r)
		if err != nil {
			return nil, err
		}
		var size int
		switch t {
		case vdfMap:
			if m[key], err = readVDFMap(r, depth+1); err != nil {
				return nil, err
			}
			continue
		case vdfString:
			if m[key], err = readCString(r); err != nil {
				return nil, err
			}
			continue
		case vdfInt32:
			var v uint32
			if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
				return nil, io.ErrUnexpectedEOF
			}
			m[key] = v
			continue
		case vdfFloat, vdfPtr, vdfColor:
			size = 4
		case vdfUint64, vdfInt64:
			size = 8
		default:
			return nil, fmt.Errorf("unknown value type 0x%02x", t)
		}
		if r.Len() < size {
			return nil, io.ErrUnexpectedEOF
		}
		_, _ = r.Seek(int64(size), io.SeekCurrent)
	}
}
