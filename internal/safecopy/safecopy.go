// Package safecopy provides copy helpers for untrusted persona content. A
// persona may be shared by a third party, so the copy refuses anything that is
// not a regular file or directory (symlinks, devices, sockets) fail-closed, and
// normalizes file modes (0644 files, 0755 dirs) so two materializations are
// byte- and metadata-identical.
package safecopy

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Tree recursively copies the directory at src into dst, preserving the relative
// structure with normalized modes. Any symlink or irregular entry is rejected.
func Tree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed in persona: %s", p)
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("irregular file not allowed in persona: %s", p)
		}
		return File(p, target)
	})
}

// File copies a single regular file, creating parent dirs as needed and
// normalizing the mode to 0644. The source is lstat-checked first so a symlink
// (which os.Open would silently follow) is rejected fail-closed.
func File(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("symlink not allowed in persona: %s", src)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("irregular file not allowed in persona: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
