// Package segmentlog is an append-only log of records, divided into segment files. A record is a
// payload that the package does not look into, and a segment has a header with a meta that the
// package does not look into either: whoever uses the log decides what is in them (an engine, a
// seed). It imports only the logging package of the project.
//
// Each record has an ID, a number that the log counts from 1: the first record of a segment has the
// ID that its header says (its begin), and each record after it the next number, across segments
// too, so the IDs of a log have no gap. Whoever uses the log can make the ID mean something (a
// tick), and then need not write it in the payload.
//
// A directory holds the segments of one log, each named `seg-<index>.pbsl` with the index in 20
// digits, counted from 0 in the order the segments were made, so the names sort in that order. A
// segment is a header and then records:
//
//	header: magic "PBSL", format version u16, begin u64, meta length u32, meta,
//	        crc32 of all before it
//	record: payload length u32, crc32 of the payload, payload
//	commit: 0xFFFFFFFF u32, crc32 of the count, count u32 (the records since the last commit)
//
// Numbers are little-endian and the crc is CRC-32C. A batch (what one Commit hands over) is its
// records and then a commit, and it is all or nothing: a crash, or a process that is killed in the
// middle of a write, can leave part of a batch at the end of the last segment, and it is detected
// by the commit that is missing or wrong and cut off, so a reader never sees half of a batch.
package segmentlog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	magic         = "PBSL"
	formatVersion = 1
	maxMeta       = 1 << 16
	maxRecord     = 1 << 24
	frameHead     = 8
	commitMark    = 0xFFFFFFFF
	commitSize    = 12
	namePrefix    = "seg-"
	nameSuffix    = ".pbsl"
)

var (
	// ErrTooLarge is returned for a payload larger than 16 MiB.
	ErrTooLarge = errors.New("segmentlog: the payload is too large")
	// ErrClosed is returned by a Writer that was closed.
	ErrClosed = errors.New("segmentlog: the writer is closed")
	// ErrCorrupt is wrapped by the error of a segment that is damaged somewhere other than the end
	// of the last segment.
	ErrCorrupt = errors.New("segmentlog: the segment is corrupt")
	// ErrBadOptions is returned by Open for options that cannot work.
	ErrBadOptions = errors.New(
		"segmentlog: the segment size and the sync period must be positive and the meta no more " +
			"than 64 KiB",
	)
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// Segments lists the segments of the log in dir, in the order they were made. Files that are not
// named like a segment are ignored. A directory that does not exist has none.
func Segments(dir string) ([]Segment, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Segment
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, namePrefix) ||
			!strings.HasSuffix(name, nameSuffix) {
			continue
		}
		index, err := strconv.ParseUint(
			strings.TrimSuffix(strings.TrimPrefix(name, namePrefix), nameSuffix), 10, 64,
		)
		if err != nil {
			continue
		}
		out = append(out, Segment{Index: index, Path: filepath.Join(dir, name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// Segment is a segment file of a log: its index, counted from 0 in the order the segments were
// made.
type Segment struct {
	Index uint64
	Path  string
}

// appendHeader appends the header of a segment whose first record has the ID begin.
func appendHeader(b []byte, begin uint64, meta []byte) []byte {
	start := len(b)
	b = append(b, magic...)
	b = binary.LittleEndian.AppendUint16(b, formatVersion)
	b = binary.LittleEndian.AppendUint64(b, begin)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(meta)))
	b = append(b, meta...)
	return binary.LittleEndian.AppendUint32(b, crc32.Checksum(b[start:], castagnoli))
}

// frameChecksum is the crc of a record: of its ID, which is not written, its length and its
// payload. A record that is read at another place than the one it was written at has another ID,
// and so does not match.
func frameChecksum(id uint64, length uint32, payload []byte) uint32 {
	var head [12]byte
	binary.LittleEndian.PutUint64(head[:8], id)
	binary.LittleEndian.PutUint32(head[8:], length)
	return crc32.Update(crc32.Checksum(head[:], castagnoli), castagnoli, payload)
}

// appendFrame appends a record that has the ID id.
func appendFrame(b []byte, id uint64, payload []byte) []byte {
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	b = binary.LittleEndian.AppendUint32(b, frameChecksum(id, uint32(len(payload)), payload))
	return append(b, payload...)
}

// appendCommit appends the commit that ends a batch of count records.
func appendCommit(b []byte, count uint32) []byte {
	var c [4]byte
	binary.LittleEndian.PutUint32(c[:], count)
	b = binary.LittleEndian.AppendUint32(b, commitMark)
	b = binary.LittleEndian.AppendUint32(b, crc32.Checksum(c[:], castagnoli))
	return binary.LittleEndian.AppendUint32(b, count)
}

func segmentName(index uint64) string {
	return fmt.Sprintf("%s%020d%s", namePrefix, index, nameSuffix)
}
