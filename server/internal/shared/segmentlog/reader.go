package segmentlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

// Reader reads the records of a log in order.
type Reader struct {
	segments []Segment
	next     int // the index of the segment to open after the current one
	f        *os.File
	s        *segmentScanner
	err      error

	// The last segment may end with part of a batch that a crash cut, so its records are held back until the commit
	// of their batch is read: collecting is the records of the batch being read, ready the records of a batch whose
	// commit was read and that Next has not given yet. The other segments are read as they come.
	buffering  bool
	collecting [][]byte
	ready      [][]byte
	readyPos   int
	payload    []byte
}

// OpenReader reads the log in dir from a segment the caller chooses. start is given the meta of a segment
// and reports whether the segment is at or before where the caller wants to start; the reader starts at the
// last segment for which it is true, or at the first when it is true for none, and reads on to the end of
// the log. start has to be true for the first segments and false for the rest (a tick that is no later than
// the one wanted is that); it is called for about log2(n) of the n segments, each of which has its header
// read. A nil start reads from the first segment. A last segment whose header was not finished when a
// crash came is not in the log.
func OpenReader(dir string, start func(meta []byte) bool) (*Reader, error) {
	segments, err := Segments(dir)
	if err != nil {
		return nil, err
	}
	first := 0
	if start != nil {
		var searchErr error
		first = sort.Search(len(segments), func(i int) bool {
			meta, err := readMeta(segments[i].Path)
			if errors.Is(err, errHeaderCut) && i == len(segments)-1 {
				return true // a header that was not finished: not before anything
			}
			if err != nil && searchErr == nil {
				searchErr = fmt.Errorf("%s: %w", segments[i].Path, err)
			}
			return err != nil || !start(meta)
		})
		if searchErr != nil {
			return nil, searchErr
		}
		first = max(first-1, 0)
	}
	return &Reader{segments: segments, next: first}, nil
}

// readMeta reads the meta of the header of a segment.
func readMeta(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := newSegmentScanner(f)
	if err != nil {
		return nil, err
	}
	return s.meta, nil
}

// Next moves to the next record and reports whether there is one. It reports false at the end of the log
// and when there is an error, which Err returns. What follows the last whole batch of the last segment (a
// record cut off, or records whose commit is missing) is the end of the log, not an error: it is what a
// crash leaves.
func (r *Reader) Next() bool {
	for r.err == nil {
		if r.readyPos < len(r.ready) {
			r.payload = r.ready[r.readyPos]
			r.readyPos++
			return true
		}
		if r.s == nil && !r.openNext() {
			return false
		}
		err := r.s.next()
		switch {
		case err == nil && r.s.commit:
			// the end of a batch: its records are whole, and the next calls give them
			r.ready, r.collecting = r.collecting, r.ready[:0]
			r.readyPos = 0
		case err == nil && r.buffering:
			r.collecting = append(r.collecting, bytes.Clone(r.s.payload))
		case err == nil:
			r.payload = r.s.payload
			return true
		case errors.Is(err, io.EOF), errors.Is(err, errTorn) && r.buffering:
			// the end of the segment, or of what a crash left of the last one: the records of a batch that was not
			// committed are dropped
			r.closeFile()
			if r.buffering {
				r.next = len(r.segments)
			}
		default:
			r.err = fmt.Errorf("%s: %w", r.segments[r.next-1].Path, err)
		}
	}
	return false
}

func (r *Reader) openNext() bool {
	if r.next >= len(r.segments) {
		return false
	}
	seg := r.segments[r.next]
	f, err := os.Open(seg.Path)
	if err != nil {
		r.err = err
		return false
	}
	s, err := newSegmentScanner(f)
	last := r.next == len(r.segments)-1
	if err != nil {
		f.Close()
		if last && errors.Is(err, errHeaderCut) { // made but not finished when the crash came
			r.next++
			return false
		}
		r.err = fmt.Errorf("%s: %w", seg.Path, err)
		return false
	}
	r.f, r.s, r.buffering = f, s, last
	r.collecting, r.ready, r.readyPos = r.collecting[:0], r.ready[:0], 0
	r.next++
	return true
}

func (r *Reader) closeFile() {
	if r.f != nil {
		r.f.Close()
	}
	r.f, r.s = nil, nil
}

// Payload is the payload of the current record. It is valid until the next call of Next.
func (r *Reader) Payload() []byte { return r.payload }

// SegmentIndex is the index of the segment of the current record.
func (r *Reader) SegmentIndex() uint64 { return r.segments[r.next-1].Index }

// Meta is the meta of the header of the segment of the current record.
func (r *Reader) Meta() []byte { return r.s.meta }

// Err is the error that stopped Next, or nil.
func (r *Reader) Err() error { return r.err }

// Close releases the file that is open.
func (r *Reader) Close() error {
	r.closeFile()
	return nil
}
