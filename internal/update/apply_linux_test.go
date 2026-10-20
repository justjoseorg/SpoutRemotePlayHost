package update

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractBinary(t *testing.T) {
	dir := t.TempDir()
	tarball := filepath.Join(dir, "pkg.tar.gz")
	f, _ := os.Create(tarball)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range []struct{ name, body string }{{"./install.sh", "sh"}, {"./sub/spout-host", "wrong"}, {"./spout-host", "bin"}} {
		_ = tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(e.body))
	}
	tw.Close()
	gz.Close()
	f.Close()
	dst := filepath.Join(dir, "out")
	if err := extractBinary(tarball, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "bin" {
		t.Fatalf("got %q", b)
	}
	if st, _ := os.Stat(dst); st.Mode().Perm()&0o100 == 0 {
		t.Fatal("not executable")
	}
}
