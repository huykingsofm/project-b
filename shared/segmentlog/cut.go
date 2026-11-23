package segmentlog

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// CutAfter cuts the log in dir after the last batch whose records keep says are all to be kept:
// the first record that keep refuses, and everything after it, is cut. The segment that holds it
// is cut at the start of its batch, and the segments after it are taken out of the log. Nothing is
// thrown away: what is cut is kept next to the segment, in a file named
// <segment>.discarded-<offset> (a segment that is taken out whole is renamed so, with the offset
// 0), which Segments does not see, and a warning says so. A log in which keep refuses nothing is
// not changed. The log must not be open for writing: a Writer is opened on the log that is left.
//
// It is what a recovery does to the log of an engine whose last ticks were lost to a crash that
// another engine, which depends on them, did not lose: both go back to the last tick that no crash
// takes back.
func CutAfter(dir string, keep func(payload []byte) bool) error {
	segments, err := Segments(dir)
	if err != nil {
		return err
	}
	for i, seg := range segments {
		at, whole, err := firstRefused(seg.Path, keep)
		if err != nil {
			return err
		}
		if at < 0 {
			continue // all of it is kept
		}
		if whole {
			if err := discardSegment(dir, seg.Path); err != nil {
				return err
			}
		} else if err := cutSegment(dir, seg.Path, at); err != nil {
			return err
		}
		for _, later := range segments[i+1:] {
			if err := discardSegment(dir, later.Path); err != nil {
				return err
			}
		}
		return syncDir(dir)
	}
	return nil
}

// firstRefused finds where a segment is to be cut: the start of the batch that holds the first
// record that keep refuses, or -1 when it refuses none. whole says that nothing of the segment is
// kept. The end of the last segment, which a crash can leave torn, is not looked at: what is whole
// is.
func firstRefused(path string, keep func([]byte) bool) (at int64, whole bool, err error) {
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
		if !s.commit && !keep(s.payload) {
			return s.committedEnd, s.committedEnd == headerEnd, nil
		}
	}
}

// cutSegment cuts a segment at an offset, keeping what is cut.
func cutSegment(dir, path string, at int64) error {
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
		if err := keepDiscarded(dir, f, path, at, size); err != nil {
			return err
		}
	}
	if err := f.Truncate(at); err != nil {
		return err
	}
	return f.Sync()
}

// discardSegment takes a segment out of the log without throwing it away.
func discardSegment(dir, path string) error {
	name := fmt.Sprintf("%s.discarded-%d", path, 0)
	if err := os.Rename(path, name); err != nil {
		return err
	}
	return syncDir(dir)
}
