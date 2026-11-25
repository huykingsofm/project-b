package segmentlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/huykingsofm/project-b/shared/atomicfile"
)

// CutAfter cuts the log in dir after the record with the ID limit: the records whose ID is greater
// than limit are cut. It looks at the headers of about log2(n) of the n segments to find the one
// that holds the record after limit, and reads only that one. The segment is cut at the start of
// the batch of the first record that goes, which can take with it records of that batch that were
// to be kept, and the segments after it are taken out of the log. Nothing is thrown away: what is
// cut is kept in archiveDir, in a file named <segment>.discarded-<offset> (a segment that is taken
// out whole is moved there with the offset 0), where Segments does not see it, and a warning says
// so. archiveDir is made when it is needed, and must be on the same volume as dir. A log with no
// record after limit is not changed. The log must not be open for writing: a Writer is opened on
// the log that is left, and the ID of its next record is limit + 1.
//
// It is what a recovery does to the log of an engine whose last ticks were lost to a crash that
// another engine, which depends on them, did not lose: both go back to the last tick that no crash
// takes back.
func CutAfter(dir string, limit uint64, archiveDir string) error {
	segments, err := Segments(dir)
	if err != nil {
		return err
	}
	holder, err := lastSegmentBeginningAtOrBefore(segments, limit)
	if err != nil {
		return err
	}
	firstOut := holder + 1 // the first segment that is taken out whole
	cutAt := int64(-1)     // where the segment of holder is cut, or -1 when all of it is kept
	if holder >= 0 {
		at, whole, err := firstRecordAfter(segments[holder].Path, limit)
		if err != nil {
			return err
		}
		if whole {
			firstOut, cutAt = holder, -1
		} else {
			cutAt = at
		}
	}
	if cutAt < 0 && firstOut == len(segments) {
		return nil
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	// from the end, so that a crash in the middle leaves a log that is whole and longer than it
	// should be, and not one with a hole: the cut is made again
	for i := len(segments) - 1; i >= firstOut; i-- {
		if err := discardSegment(archiveDir, segments[i].Path); err != nil {
			return err
		}
	}
	if cutAt >= 0 {
		if err := cutSegment(archiveDir, segments[holder].Path, cutAt); err != nil {
			return err
		}
	}
	return atomicfile.SyncDir(dir)
}

// firstRecordAfter finds where a segment is to be cut: the start of the batch that holds the first
// record whose ID is greater than limit, or -1 when it has none. whole says that nothing of the
// segment is kept. The end of the last segment, which a crash can leave torn, is not looked at:
// what is whole is.
func firstRecordAfter(path string, limit uint64) (at int64, whole bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return -1, false, err
	}
	defer f.Close()
	s, err := newSegmentScanner(f)
	if errors.Is(err, errHeaderCut) {
		return 0, true, nil // a segment that never got a header holds nothing to keep
	}
	if err != nil {
		return -1, false, fmt.Errorf("%s: %w", path, err)
	}
	headerEnd := s.committedEnd
	for {
		if err := s.next(); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, errTorn) {
				return -1, false, nil
			}
			return -1, false, fmt.Errorf("%s: %w", path, err)
		}
		if !s.commit && s.recordID() > limit {
			return s.committedEnd, s.committedEnd == headerEnd, nil
		}
	}
}

// cutSegment cuts a segment at an offset, keeping what is cut in archiveDir.
func cutSegment(archiveDir, path string, at int64) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if size > at {
		if err := keepDiscarded(archiveDir, f, path, at, size); err != nil {
			return err
		}
	}
	if err := f.Truncate(at); err != nil {
		return err
	}
	return f.Sync()
}

// discardSegment takes a segment out of the log without throwing it away: it is moved to
// archiveDir.
func discardSegment(archiveDir, path string) error {
	name := filepath.Join(archiveDir, fmt.Sprintf("%s.discarded-%d", filepath.Base(path), 0))
	if err := os.Rename(path, name); err != nil {
		return err
	}
	if err := atomicfile.SyncDir(archiveDir); err != nil {
		return err
	}
	return atomicfile.SyncDir(filepath.Dir(path))
}
