package segmentlog

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// segmentScanner reads the header and then the records of a segment.
type segmentScanner struct {
	r       *bufio.Reader
	begin   uint64 // the ID of the first record of the segment
	meta    []byte
	offset  int64 // where the next entry starts: the end of what has been read whole
	payload []byte
	buf     []byte
	commit  bool // the entry that next read is a commit, not a record
	// committedEnd is where the last commit that was read ends, or the end of the header when there
	// was none: what is before it is whole batches. sinceCommit is the records read since that
	// commit, and committedRecords the records in the batches that were committed.
	committedEnd     int64
	sinceCommit      int
	committedRecords int64
}

// errTorn says that the end of the segment is a record that was not finished or does not match its
// crc; at the end of the last segment that is what a crash leaves.
var errTorn = fmt.Errorf("%w: a record is cut off or does not match its crc", ErrCorrupt)

// errHeaderCut says that the file ends inside the header, or holds only zeros where the header
// should be: a segment that never got a header, which is what a crash leaves of a file that was
// just made (a file system can keep the size of the file and zeros for the data that was not
// written). Any other header that is not right, another format or a crc that does not match, is not
// cut, and what is in the file is not touched.
var errHeaderCut = fmt.Errorf("%w: the header is cut off", ErrCorrupt)

func newSegmentScanner(r io.Reader) (*segmentScanner, error) {
	s := &segmentScanner{r: bufio.NewReaderSize(r, 64<<10)}
	var head [18]byte
	if _, err := io.ReadFull(s.r, head[:]); err != nil {
		return nil, errHeaderCut
	}
	if string(head[:4]) != magic || binary.LittleEndian.Uint16(head[4:6]) != formatVersion {
		if head == [len(head)]byte{} {
			return nil, errHeaderCut
		}
		return nil, fmt.Errorf("%w: not a segment of this format", ErrCorrupt)
	}
	s.begin = binary.LittleEndian.Uint64(head[6:14])
	metaLen := binary.LittleEndian.Uint32(head[14:18])
	if metaLen > maxMeta {
		return nil, fmt.Errorf("%w: the meta is %d bytes", ErrCorrupt, metaLen)
	}
	rest := make([]byte, int(metaLen)+4)
	if _, err := io.ReadFull(s.r, rest); err != nil {
		return nil, errHeaderCut
	}
	s.meta = rest[:metaLen]
	sum := crc32.Update(crc32.Checksum(head[:], castagnoli), castagnoli, s.meta)
	if sum != binary.LittleEndian.Uint32(rest[metaLen:]) {
		return nil, fmt.Errorf("%w: the header does not match its crc", ErrCorrupt)
	}
	s.offset = int64(len(head) + len(rest))
	s.committedEnd = s.offset
	return s, nil
}

// recordID is the ID of the record that next read last, which is not a commit.
func (s *segmentScanner) recordID() uint64 {
	return s.begin + uint64(s.committedRecords) + uint64(s.sinceCommit) - 1
}

// next reads the next entry, a record or a commit (s.commit says which). It returns io.EOF at the
// end of the segment, and errTorn when the end is an entry that is not whole, or a commit that does
// not match the records before it.
func (s *segmentScanner) next() error {
	s.commit = false
	var head [frameHead]byte
	if _, err := io.ReadFull(s.r, head[:]); err == io.EOF {
		return io.EOF
	} else if err != nil {
		return errTorn
	}
	length := binary.LittleEndian.Uint32(head[:4])
	if length == commitMark {
		var count [4]byte
		if _, err := io.ReadFull(s.r, count[:]); err != nil {
			return errTorn
		}
		n := binary.LittleEndian.Uint32(count[:])
		if crc32.Checksum(count[:], castagnoli) != binary.LittleEndian.Uint32(head[4:]) ||
			int(n) != s.sinceCommit {
			return errTorn
		}
		s.offset += commitSize
		s.committedEnd = s.offset
		s.committedRecords += int64(n)
		s.sinceCommit = 0
		s.commit = true
		return nil
	}
	if length > maxRecord {
		return errTorn
	}
	if cap(s.buf) < int(length) {
		s.buf = make([]byte, length)
	}
	s.buf = s.buf[:length]
	if _, err := io.ReadFull(s.r, s.buf); err != nil {
		return errTorn
	}
	id := s.begin + uint64(s.committedRecords) + uint64(s.sinceCommit)
	if frameChecksum(id, length, s.buf) != binary.LittleEndian.Uint32(head[4:]) {
		return errTorn
	}
	s.payload = s.buf
	s.offset += int64(frameHead) + int64(length)
	s.sinceCommit++
	return nil
}
