package segmentlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
)

// Reader reads the records of a log in order.
type Reader struct {
	segments []Segment
	next     int // the index of the segment to open after the current one
	f        *os.File
	s        *segmentScanner
	err      error

	// The last segment may end with part of a batch that a crash cut, so its records are held back
	// until the commit of their batch is read: collecting is the records of the batch being read,
	// ready the records of a batch whose commit was read and that Next has not given yet. The other
	// segments are read as they come.
	buffering  bool
	collecting [][]byte
	ready      [][]byte
	readyPos   int
	payload    []byte

	from   uint64 // the ID of the first record that Next gives
	id     uint64 // the ID of the current record
	nextID uint64 // the ID of the record that is read next
}

// OpenReader reads the log in dir from the record with the ID from: it starts at the last segment
// that begins at or before from, or at the first when none does, and reads on to the end of the
// log. It looks at the headers of about log2(n) of the n segments and reads no record of the ones
// before. The records of that segment that come before from are not given. A last segment whose
// header was not finished when a crash came is not in the log.
func OpenReader(dir string, from uint64) (*Reader, error) {
	segments, err := Segments(dir)
	if err != nil {
		return nil, err
	}
	first, err := lastSegmentBeginningAtOrBefore(segments, from)
	if err != nil {
		return nil, err
	}
	return &Reader{segments: segments, next: max(first, 0), from: from}, nil
}

// lastSegmentBeginningAtOrBefore is the position in segments of the last segment whose begin is no
// greater than id, or -1 when there is none: a binary search that reads the header of about
// log2(n) segments, since the begins grow with the index. A last segment whose header was not
// finished is not before anything.
func lastSegmentBeginningAtOrBefore(segments []Segment, id uint64) (int, error) {
	var searchErr error
	after := sort.Search(len(segments), func(i int) bool {
		begin, _, err := readHeader(segments[i].Path)
		if errors.Is(err, errHeaderCut) && i == len(segments)-1 {
			return true
		}
		if err != nil && searchErr == nil {
			searchErr = fmt.Errorf("%s: %w", segments[i].Path, err)
		}
		return err != nil || begin > id
	})
	if searchErr != nil {
		return 0, searchErr
	}
	return after - 1, nil
}

// readHeader reads the begin and the meta of the header of a segment.
func readHeader(path string) (begin uint64, meta []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	s, err := newSegmentScanner(f)
	if err != nil {
		return 0, nil, err
	}
	return s.begin, s.meta, nil
}

// Next moves to the next record, which has an ID of from or more, and reports whether there is
// one. It reports false at the end of the log and when there is an error, which Err returns. What
// follows the last whole batch of the last segment (a record cut off, or records whose commit is
// missing) is the end of the log, not an error: it is what a crash leaves.
func (r *Reader) Next() bool {
	for r.step() {
		if r.id >= r.from {
			return true
		}
	}
	return false
}

// step moves to the next record, whatever its ID.
func (r *Reader) step() bool {
	for r.err == nil {
		if r.readyPos < len(r.ready) {
			r.payload = r.ready[r.readyPos]
			r.readyPos++
			r.id, r.nextID = r.nextID, r.nextID+1
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
			r.id, r.nextID = r.nextID, r.nextID+1
			return true
		case errors.Is(err, io.EOF), errors.Is(err, errTorn) && r.buffering:
			// the end of the segment, or of what a crash left of the last one: the records of a
			// batch that was not committed are dropped
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
	r.nextID = s.begin
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

// ID is the ID of the current record.
func (r *Reader) ID() uint64 { return r.id }

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

// LastFrameID is the ID of the last record of the log in dir, or 0 when it has none. It reads the
// last segment that has a whole batch, which is the last one unless a crash cut its header or all
// its batches, and no other, so it does not depend on the length of the log.
func LastFrameID(dir string) (uint64, error) {
	segments, err := Segments(dir)
	if err != nil {
		return 0, err
	}
	for i, segment := range slices.Backward(segments) {
		last := i == len(segments)-1
		f, err := os.Open(segment.Path)
		if err != nil {
			return 0, err
		}
		s, err := newSegmentScanner(f)
		if errors.Is(err, errHeaderCut) && last {
			f.Close()
			continue
		}
		if err != nil {
			f.Close()
			return 0, fmt.Errorf("%s: %w", segment.Path, err)
		}
		for {
			if err = s.next(); err != nil {
				break
			}
		}
		f.Close()
		if !errors.Is(err, io.EOF) && !(errors.Is(err, errTorn) && last) {
			return 0, fmt.Errorf("%s: %w", segment.Path, err)
		}
		if s.committedRecords > 0 {
			return s.begin + uint64(s.committedRecords) - 1, nil
		}
	}
	return 0, nil
}
