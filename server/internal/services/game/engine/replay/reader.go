package replay

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/shared/segmentlog"
)

// FileLogReader reads the records of the log of one engine, in the order they were written.
type FileLogReader struct {
	engine    action.Address
	r         *segmentlog.Reader
	fromTick  uint64
	rec       Record
	info      segmentInfo // of the segment of the current record
	infoIndex uint64
	haveInfo  bool
	err       error
}

// ReadFileLog reads the log that a FileLog wrote under dir for an engine, from fromTick: it starts
// in the segment that holds that tick, using the meta of the segments, and gives only the records
// of that tick and later ones.
func ReadFileLog(dir string, engine action.Address, fromTick uint64) (*FileLogReader, error) {
	start := func(meta []byte) bool {
		info, err := parseSegmentMeta(meta)
		return err == nil && info.FirstTick <= fromTick
	}
	r, err := segmentlog.OpenReader(engineDir(dir, engine), start)
	if err != nil {
		return nil, err
	}
	return &FileLogReader{engine: engine, r: r, fromTick: fromTick}, nil
}

// Next moves to the next record and reports whether there is one. It reports false at the end of
// the log, and when there is an error, which Err returns.
func (l *FileLogReader) Next() bool {
	for l.err == nil && l.r.Next() {
		if index := l.r.SegmentIndex(); !l.haveInfo || index != l.infoIndex {
			info, err := parseSegmentMeta(l.r.Meta())
			if err != nil {
				l.err = fmt.Errorf("segment %d: %w", index, err)
				return false
			}
			if info.Engine != l.engine {
				l.err = fmt.Errorf(
					"segment %d is the log of %v, not of %v", index, info.Engine, l.engine,
				)
				return false
			}
			l.info, l.infoIndex, l.haveInfo = info, index, true
		}
		rec, err := decodeRecord(l.engine, l.info.Version, l.r.Payload())
		if err != nil {
			l.err = fmt.Errorf("segment %d: %w", l.r.SegmentIndex(), err)
			return false
		}
		if rec.Tick < l.fromTick {
			continue
		}
		l.rec = rec
		return true
	}
	if l.err == nil {
		l.err = l.r.Err()
	}
	return false
}

// Record is the current record.
func (l *FileLogReader) Record() Record { return l.rec }

// Seed is the seed of the cluster, from the meta of the segment of the current record.
func (l *FileLogReader) Seed() uint64 { return l.info.Seed }

// Err is the error that stopped Next, or nil.
func (l *FileLogReader) Err() error { return l.err }

// Close releases the files that are open.
func (l *FileLogReader) Close() error { return l.r.Close() }
