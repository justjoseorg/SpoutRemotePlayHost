package update

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// apply replaces the running binary with the one in the release tarball and restarts in place
// (same PID, so a systemd service keeps tracking it). Only the binary is updated; re-run
// install.sh from a new tarball for driver or service changes.
func apply(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	tmp := exe + ".new"
	if err := extractBinary(path, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", exe, err)
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}

// extractBinary writes the tarball's spout-host to dst.
func extractBinary(tarball, dst string) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("the update package has no spout-host binary")
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || filepath.Base(h.Name) != "spout-host" || filepath.Dir(filepath.Clean(h.Name)) != "." {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, io.LimitReader(tr, 256<<20))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

// RunSetup is only used on Windows.
func RunSetup(string) error { return errors.New("not supported on this system") }
