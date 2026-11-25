// Package atomicfile writes a file so that it is on the disk whole, or not there at all, and syncs
// directories so that what was made or renamed in them stays after a crash. It imports nothing of
// the project.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// tmpSuffix is added to the name of the file while it is written. A file left by a crash is not the
// file, and is written over by the next Write of the same path.
const tmpSuffix = ".tmp"

// Write writes data to path: to a temporary file next to it, which is synced and then renamed, and
// then the directory is synced, so that when it returns the file is on the disk whole. The
// directory of the path is made when it is not there. A file already at path is replaced whole.
func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + tmpSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return SyncDir(dir)
}

// SyncDir forces the entries of a directory to disk: the file that was made, renamed or deleted in
// it is not durable until then, and after a power loss a file that was synced could be missing from
// its directory.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		d.Close()
		return fmt.Errorf("sync of the directory %s: %w", dir, err)
	}
	return d.Close()
}
