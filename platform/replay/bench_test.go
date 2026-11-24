package replay

import (
	"fmt"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// benchMessage is a message for a zonelayer with two actions, as the engines send them.
func benchMessage(seq uint64) sim.Message {
	return sim.NewActionMessage[*toyState](
		3, onfail.Drop{},
		toyMoveAction{CharacterID: 7, Dir: 1, Ticks: 3},
		toyChatAction{FromID: 7, Text: "xin chào mọi người"},
	).Stamp(sim.Header{From: sim.StateID(11), Seq: seq})
}

var benchRecords = map[string]Record{
	"inbound, 2 actions": {Tick: 123456, Body: Inbound{Message: benchMessage(98765)}},
	"command, move": {
		Tick: 123456,
		Body: Command{
			Command: toyMove{
				CharacterID: 7, Dir: 1, Ticks: 3,
			},
		},
	},
	"command, chat": {
		Tick: 123456,
		Body: Command{
			Command: toyChat{
				FromID: 7, Text: "xin chào mọi người, hôm nay thế nào",
			},
		},
	},
	"hash":        {Tick: 123456, Body: Hash{Value: 0xdeadbeefcafef00d}},
	"config read": {Tick: 123456, Body: ConfigRead{Version: 5}},
	"stable read": {
		Tick: 123456, Body: StableRead{Source: sim.StateID(12), CopyTick: 123455},
	},
}

// BenchmarkEncodeRecord is the part of recording that the goroutine of an engine does: building the
// bytes of a record. There is no file and no writer.
func BenchmarkEncodeRecord(b *testing.B) {
	for name, rec := range benchRecords {
		b.Run(name, func(b *testing.B) {
			var w binary.Writer
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				w.B = w.B[:0]
				if err := encodeRecord(&w, rec.Body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecodeRecord(b *testing.B) {
	engine, codecs := sim.StateID(1), toyCodecs()
	for name, rec := range benchRecords {
		var w binary.Writer
		if err := encodeRecord(&w, rec.Body); err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := decodeRecord(codecs, engine, formatVersion, rec.Tick, w.B)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// tickRecorder records the ticks of one engine: a config read, a stable read, a hash, 5 commands
// and the rest inbound messages, as a busy zonelayer does.
type tickRecorder struct {
	log     *FileLog
	engine  sim.StateID
	records int
	// already an interface value, as a command is when an engine records it
	move sim.BaseCommand
	// made once: what is measured is the log, not making a message
	message sim.Message
}

func newTickRecorder(log *FileLog, engine sim.StateID, records int) *tickRecorder {
	return &tickRecorder{
		log: log, engine: engine, records: records,
		move:    toyMove{CharacterID: 7, Dir: 1, Ticks: 3},
		message: benchMessage(1),
	}
}

func (r *tickRecorder) tick(tick uint64) {
	r.log.Append(r.engine, tick, ConfigRead{Version: 5})
	for range r.records - 8 {
		r.log.Append(r.engine, tick, Inbound{Message: r.message})
	}
	for range 5 {
		r.log.Append(r.engine, tick, Command{Command: r.move})
	}
	r.log.Append(r.engine, tick, StableRead{Source: sim.StateID(11), CopyTick: tick - 1})
	r.log.Append(r.engine, tick, Hash{Value: 42})
	r.log.EndTick(r.engine, tick)
}

func benchFile(b *testing.B) *FileLog {
	return NewFileLog(
		b.TempDir(),
		FileLogOptions{Seed: 1, SegmentBytes: 64 << 20, SyncEvery: time.Second, Backlog: 64},
	)
}

// stalls is how many times a Commit of the log of an engine had to wait for the goroutine that
// writes.
func stalls(log *FileLog, engine sim.StateID) uint64 {
	v, ok := log.engines.Load(engine)
	if !ok {
		return 0
	}
	return v.(*engineLog).w.Stalled()
}

// percentiles reports the time of the ticks as seen by the engine: the median, the 99th percentile
// and the worst.
func percentiles(b *testing.B, durs []time.Duration) {
	slices.Sort(durs)
	at := func(q float64) float64 {
		return float64(durs[int(float64(len(durs)-1)*q)].Nanoseconds())
	}
	b.ReportMetric(at(0.5), "p50-ns/tick")
	b.ReportMetric(at(0.99), "p99-ns/tick")
	b.ReportMetric(float64(durs[len(durs)-1].Nanoseconds()), "max-ns/tick")
}

// BenchmarkFileTickHot records ticks one after the other with nothing in between, so everything is
// in the cache and the goroutine that writes is never asked to wait for: it is the least that a
// tick can cost, and not what an engine pays at 50 ticks a second.
func BenchmarkFileTickHot(b *testing.B) {
	log := benchFile(b)
	r := newTickRecorder(log, sim.StateID(1), 20)
	b.ReportAllocs()
	for tick := uint64(1); tick <= uint64(b.N); tick++ {
		r.tick(tick)
	}
	b.StopTimer()
	if err := log.Close(); err != nil {
		b.Fatal(err)
	}
}

// pacedTicks and pacedPause are the workload of BenchmarkFileTickPaced: 3,000 ticks with a
// millisecond between them.
const (
	pacedTicks = 3000
	pacedPause = time.Millisecond
)

// BenchmarkFileTickPaced is what an engine pays: a tick of 20, 100 or 1,000 records, a pause
// between ticks as the engine has between its own work, and the time of each tick measured from the
// engine's side. The workload is fixed (pacedTicks ticks), so ns/op is the cost of the whole run
// and means nothing: the metrics are the mean, the median, the 99th percentile and the worst of a
// tick, in ns/tick, and the number of waits for the writer.
func BenchmarkFileTickPaced(b *testing.B) {
	for _, records := range []int{20, 100, 1000} {
		b.Run(fmt.Sprintf("records=%d", records), func(b *testing.B) {
			log := benchFile(b)
			engine := sim.StateID(1)
			r := newTickRecorder(log, engine, records)
			durs := make([]time.Duration, 0, pacedTicks)
			var total time.Duration
			for tick := uint64(1); tick <= pacedTicks; tick++ {
				t0 := time.Now()
				r.tick(tick)
				d := time.Since(t0)
				durs = append(durs, d)
				total += d
				time.Sleep(pacedPause)
			}
			b.ReportMetric(float64(total.Nanoseconds())/pacedTicks, "mean-ns/tick")
			percentiles(b, durs)
			b.ReportMetric(float64(stalls(log, engine)), "stalls")
			if err := log.Close(); err != nil {
				b.Fatal(err)
			}
		})
	}
}

// BenchmarkFileTickSaturated records ticks of 100 records with no pause, far faster than an engine
// can tick, so the goroutine that writes falls behind and a Commit has to wait for it: the median,
// the worst and the number of waits show what happens to an engine when the disk cannot keep up
// (the sync runs on the goroutine that writes).
func BenchmarkFileTickSaturated(b *testing.B) {
	log := benchFile(b)
	engine := sim.StateID(1)
	r := newTickRecorder(log, engine, 100)
	durs := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	for tick := uint64(1); tick <= uint64(b.N); tick++ {
		t0 := time.Now()
		r.tick(tick)
		durs = append(durs, time.Since(t0))
	}
	b.StopTimer()
	percentiles(b, durs)
	b.ReportMetric(float64(stalls(log, engine)), "stalls")
	closing := time.Now()
	if err := log.Close(); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(time.Since(closing).Nanoseconds()), "close-ns")
}

// BenchmarkFileTickManyEngines is 12 engines, each on its own goroutine with its own log, recording
// ticks of 100 records with no pause: the writers share the disk, as the engines of a cluster do.
// ns/op is a tick of any engine.
func BenchmarkFileTickManyEngines(b *testing.B) {
	log := benchFile(b)
	var next atomic.Uint32
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		r := newTickRecorder(log, sim.StateID(next.Add(1)), 100)
		for tick := uint64(1); pb.Next(); tick++ {
			r.tick(tick)
		}
	})
	b.StopTimer()
	var total uint64
	for id := uint32(1); id <= next.Load(); id++ {
		total += stalls(log, sim.StateID(id))
	}
	b.ReportMetric(float64(total), "stalls")
	if err := log.Close(); err != nil {
		b.Fatal(err)
	}
}
