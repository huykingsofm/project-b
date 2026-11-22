package runner

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/replay"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
)

// verifyLog is the replay log of a replay: it checks that the engine records, in the same order,
// exactly what it recorded the first time. A hash is worked out and compared.
type verifyLog struct {
	want map[message.Address][]replay.Record
	pos  map[message.Address]int
	div  *divergences
}

func (v *verifyLog) expect(engine message.Address, got replay.Record) {
	records, i := v.want[engine], v.pos[engine]
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

// Wants reports true for every kind: the replay works out a hash to compare it.
func (v *verifyLog) Wants(replay.Kind) bool { return true }

// Append checks what the engine records against the next record of its log.
func (v *verifyLog) Append(e message.Address, tick uint64, body replay.Body) {
	v.expect(e, replay.Record{Tick: tick, Body: body})
}

// EndTick checks that the engine made every record that the log has for the tick.
func (v *verifyLog) EndTick(e message.Address, tick uint64) {
	records, i := v.want[e], v.pos[e]
	if i < len(records) && records[i].Tick <= tick {
		v.div.add(e, "tick %d: the log has a %T that the replay did not make",
			records[i].Tick, records[i].Body)
		v.pos[e] = len(records)
	}
}
