package segmentlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/huykingsofm/project-b/shared/logging"
)

// repair makes the last segment of the log in dir whole after a crash. What follows the last whole
// batch (a record that is cut off, or records whose commit is missing) is cut from the file, and a
// segment with no whole batch is deleted, and so is one whose header was never written (cut off, or
// zeros). Only the last segment can have been written when the crash came; damage anywhere else is
// not repaired and is reported when the segment is read.
//
// What repair removes is not thrown away: the bytes that were cut are kept next to the segment, in
// a file named <segment>.discarded-<offset>, and a warning says so, because the bytes after the
// last whole batch are a batch that a crash cut or whole batches after damage in the middle of the
// segment, and repair cannot tell the two apart. A segment whose header is of another format, or
// whose header does not match its crc, is not repaired: Open returns the error and the file is left
// as it is.
func repair(dir string) error {
	segments, err := Segments(dir)
	if err != nil || len(segments) == 0 {
		return err
	}
	last := segments[len(segments)-1]
	f, err := os.OpenFile(last.Path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := newSegmentScanner(f)
	if errors.Is(err, errHeaderCut) {
		return removeSegment(dir, last.Path)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", last.Path, err)
	}
	for {
		if err = s.next(); err != nil {
			break
		}
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, errTorn) {
		return err
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if size > s.committedEnd {
		if err := keepDiscarded(dir, f, last.Path, s.committedEnd, size); err != nil {
			return err
		}
	}
	if s.committedRecords == 0 {
		return removeSegment(dir, last.Path)
	}
	if size != s.committedEnd {
		if err := f.Truncate(s.committedEnd); err != nil {
			return err
		}
		return f.Sync()
	}
	return nil
}

// keepDiscarded copies the bytes of a segment from `from` to `to` to a file next to it, and warns.
func keepDiscarded(dir string, f *os.File, path string, from, to int64) error {
	name := fmt.Sprintf("%s.discarded-%d", path, from)
	out, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.NewSectionReader(f, from, to-from)); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	logging.Warn(
		"segmentlog cut what follows the last whole batch of a segment and kept it",
		"segment",
		filepath.Base(path),
		"from",
		from,
		"bytes",
		to-from,
		"kept_in",
		filepath.Base(name),
	)
	return syncDir(dir)
}

// removeSegment deletes a segment and makes the deletion durable.
func removeSegment(dir, path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDir(dir)
}

// syncDir forces the entries of a directory to disk: the file that was made, or deleted, in it is
// not durable until then, and after a power loss a segment that was synced could be missing from
// its directory.
func syncDir(dir string) error {
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
