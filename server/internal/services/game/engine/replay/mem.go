package replay

import (
	"sync"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
)

// MemLog keeps the records in memory.
type MemLog struct {
	mu      sync.Mutex
	Records []Record
}

func (j *MemLog) add(r Record) {
	j.mu.Lock()
	j.Records = append(j.Records, r)
	j.mu.Unlock()
}

func (j *MemLog) Inbound(a action.Address, t uint64, h action.Header, ids []action.ID) {
	j.add(Record{Kind: KindInbound, Engine: a, Tick: t, Header: h, Actions: ids})
}

func (j *MemLog) WorldCommand(a action.Address, t uint64, c command.WorldCommand) {
	j.add(Record{Kind: KindCommand, Engine: a, Tick: t, Command: c})
}

func (j *MemLog) ZoneCommand(a action.Address, t uint64, c command.ZoneCommand) {
	j.add(Record{Kind: KindCommand, Engine: a, Tick: t, Command: c})
}

func (j *MemLog) ZoneLayerCommand(a action.Address, t uint64, c command.ZoneLayerCommand) {
	j.add(Record{Kind: KindCommand, Engine: a, Tick: t, Command: c})
}

func (j *MemLog) ConfigRead(a action.Address, t uint64, v uint64) {
	j.add(Record{Kind: KindConfig, Engine: a, Tick: t, Value: v})
}

func (j *MemLog) Hash(a action.Address, t uint64, hash func() uint64) {
	j.add(Record{Kind: KindHash, Engine: a, Tick: t, Value: hash()})
}

func (j *MemLog) StableRead(a action.Address, t uint64, from action.Address, copyTick uint64) {
	j.add(Record{Kind: KindStableRead, Engine: a, Tick: t, Source: from, Value: copyTick})
}

// EndTick does nothing: a MemLog has already kept every record.
func (j *MemLog) EndTick(action.Address, uint64) {}
