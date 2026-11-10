package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WithinDir reports whether path is a file strictly inside dir.
// Both paths must be absolute. Symlink parents are not resolved; callers
// that need that guarantee should Lstat the final path themselves.
func WithinDir(dir, path string) bool {
	if dir == "" || path == "" {
		return false
	}
	dir = filepath.Clean(dir)
	path = filepath.Clean(path)
	if !filepath.IsAbs(dir) || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ReadRegularFile reads a regular file, refusing symlinks and other modes.
// maxBytes <= 0 means no limit. A file larger than maxBytes is an error.
func ReadRegularFile(path string, maxBytes int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if maxBytes > 0 && fi.Size() > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	if maxBytes > 0 {
		r = io.LimitReader(f, maxBytes)
	}
	return io.ReadAll(r)
}
