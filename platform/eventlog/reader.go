package eventlog

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/segmentlog"
)

// FileLogReader reads the records of the log of one engine, in the order they were written.
type FileLogReader struct {
	engine    sim.StateID
	codecs    *sim.Codecs
	r         *segmentlog.Reader
	rec       Record
	frame     *binary.Reader // what is left of the frame that the current record is in
	frameTick uint64
	info      segmentInfo // of the segment of the current record
	infoIndex uint64
	haveInfo  bool
	err       error
}

// ReadFileLog reads the log that a FileLog wrote under dir for an engine, from fromTick: it starts
// in the segment that holds that tick, found by the headers of the segments, and gives only the
// records of that tick and later ones.
func ReadFileLog(
	dir string, engine sim.StateID, fromTick uint64, codecs *sim.Codecs,
) (*FileLogReader, error) {
	r, err := segmentlog.OpenReader(EngineDir(dir, engine), fromTick)
	if err != nil {
		return nil, err
	}
	return &FileLogReader{engine: engine, codecs: codecs, r: r}, nil
}

// Next moves to the next record and reports whether there is one. It reports false at the end of
// the log, and when there is an error, which Err returns. A tick with no record has a frame and no
// record, so it is passed over.
func (l *FileLogReader) Next() bool {
	for l.err == nil {
		if l.frame != nil && l.frame.Left() > 0 {
			data := l.frame.Bytes()
			if err := l.frame.Err(); err != nil {
				l.err = fmt.Errorf(
					"segment %d: the frame of the tick %d: %w", l.infoIndex, l.frameTick, err,
				)
				return false
			}
			rec, err := decodeRecord(l.codecs, l.engine, l.info.Version, l.frameTick, data)
			if err != nil {
				l.err = fmt.Errorf("segment %d: the tick %d: %w", l.infoIndex, l.frameTick, err)
				return false
			}
			l.rec = rec
			return true
		}
		if !l.r.Next() {
			break
		}
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
		l.frame, l.frameTick = binary.NewReader(l.r.Payload()), l.r.ID()
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

// engineDirs returns the directories under dir that are the log of an engine: a name, a dash and
// the number of the state, as the log names them. The engine is the one that the meta of the
// first segment says, which is what tells two engines of different types with the same number
// apart. A directory with no segment is not the log of an engine.
func engineDirs(dir string) (map[sim.StateID]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[sim.StateID]string{}
	for _, e := range entries {
		dash := strings.LastIndexByte(e.Name(), '-')
		if !e.IsDir() || dash <= 0 {
			continue
		}
		info, ok := firstSegmentInfo(filepath.Join(dir, e.Name()))
		if !ok || e.Name() != engineDirName(info.Engine) {
			continue
		}
		out[info.Engine] = e.Name()
	}
	return out, nil
}

// firstSegmentInfo is what the meta of the first segment of a log says.
func firstSegmentInfo(logDir string) (segmentInfo, bool) {
	r, err := segmentlog.OpenReader(logDir, 0)
	if err != nil {
		return segmentInfo{}, false
	}
	defer r.Close()
	if !r.Next() {
		return segmentInfo{}, false
	}
	info, err := parseSegmentMeta(r.Meta())
	return info, err == nil
}

// EngineDir returns the directory of an engine under dir, which its ID names: it holds the log of
// the engine, and its checkpoints beside it. An engine that has no log has no directory, which
// reads as an empty log.
func EngineDir(dir string, engine sim.StateID) string {
	return filepath.Join(dir, engineDirName(engine))
}

// Engines returns the engines that have a log under dir, in the order of their IDs. A directory
// that is not the log of an engine is not one of them.
func Engines(dir string) ([]sim.StateID, error) {
	dirs, err := engineDirs(dir)
	if err != nil {
		return nil, err
	}
	out := make([]sim.StateID, 0, len(dirs))
	for id := range dirs {
		out = append(out, id)
	}
	slices.Sort(out)
	return out, nil
}

// ReadAll reads every record of the log of an engine under dir, in the order they were written.
func ReadAll(dir string, engine sim.StateID, codecs *sim.Codecs) ([]Record, error) {
	r, err := ReadFileLog(dir, engine, 0, codecs)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []Record
	for r.Next() {
		out = append(out, r.Record())
	}
	return out, r.Err()
}

// Source gives the records of one engine in the order they were recorded, one at a time: the log
// of a run that lasted hours does not fit in memory. A FileLogReader is one, and so is any other
// place the records of an engine can be read from, in memory or not.
type Source interface {
	Next() bool
	Record() Record
	Err() error
	Close() error
}

// Seeded is a Source that knows the seed of the run it was recorded in, as the files of a FileLog
// do. A replay refuses a log whose seed is not the one of the builder.
type Seeded interface {
	Seed() uint64
}

// FileLogs is the logs of the engines of a cluster that a FileLog wrote under a directory: the
// logs are opened one at a time and read as a replay goes on.
type FileLogs struct {
	dir     string
	codecs  *sim.Codecs
	engines []sim.StateID
}

// NewFileLogs reads the files that a FileLog wrote under dir.
func NewFileLogs(dir string, codecs *sim.Codecs) (*FileLogs, error) {
	engines, err := Engines(dir)
	if err != nil {
		return nil, err
	}
	return &FileLogs{dir: dir, codecs: codecs, engines: engines}, nil
}

// Engines returns the engines that have a log, in the order of their IDs.
func (f *FileLogs) Engines() []sim.StateID { return f.engines }

// Last returns the ID of the last frame of the log of the engine, which is its last tick.
func (f *FileLogs) Last(engine sim.StateID) (uint64, error) {
	return segmentlog.LastFrameID(EngineDir(f.dir, engine))
}

// Open returns a reader of the log of the engine from the tick from.
func (f *FileLogs) Open(engine sim.StateID, from uint64) (Source, error) {
	r, err := ReadFileLog(f.dir, engine, from, f.codecs)
	if err != nil {
		return nil, err
	}
	return r, nil // a *FileLogReader is a Source and a Seeded
}
