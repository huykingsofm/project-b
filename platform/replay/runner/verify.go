package runner

import (
	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

// tickWindow is what the log of an engine holds for the tick the engine is about to run, or runs:
// its records, and the copies that the reads among them find.
type tickWindow struct {
	tick    uint64
	records []replay.Record
	needs   []replayNeed
}

// verifyLog is the replay log of a replay: it checks that the engine records, in the same order,
// exactly what it recorded the first time. A hash is worked out and compared.
type verifyLog struct {
	windows map[sim.StateID]*tickWindow
	pos     map[sim.StateID]int // how many records of the window the engine has made
	div     *divergences
}

func (v *verifyLog) expect(engine sim.StateID, got replay.Record) {
	var records []replay.Record
	if w := v.windows[engine]; w != nil {
		records = w.records
	}
	i := v.pos[engine]
	v.pos[engine]++
	if i >= len(records) {
		v.div.add(engine,
			"tick %d: the replay recorded a %T that the log does not have", got.Tick, got.Body)
		return
	}
	if want := records[i]; want.Tick != got.Tick || !want.Body.Same(got.Body) {
		v.div.add(engine,
			"tick %d: the log has %T (%+v), the replay made %T at tick %d (%+v)",
			want.Tick, want.Body, want.Body, got.Body, got.Tick, got.Body)
	}
}

// Append checks what the engine records against the next record of its log.
func (v *verifyLog) Append(e sim.StateID, tick uint64, body replay.Body) {
	v.expect(e, replay.Record{Tick: tick, Body: body})
}

// EndTick checks that the engine made every record that the log has for the tick.
func (v *verifyLog) EndTick(e sim.StateID, _ uint64) {
	w := v.windows[e]
	if w == nil {
		return
	}
	if i := v.pos[e]; i < len(w.records) {
		v.div.add(e, "tick %d: the log has a %T that the replay did not make",
			w.records[i].Tick, w.records[i].Body)
		v.pos[e] = len(w.records)
	}
}
