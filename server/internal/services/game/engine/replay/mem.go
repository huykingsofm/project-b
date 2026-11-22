package replay

import (
	"sync"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
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

// Wants reports true for every kind: a MemLog keeps what it is given.
func (j *MemLog) Wants(Kind) bool { return true }

func (j *MemLog) Append(a action.Address, t uint64, body Body) {
	j.add(Record{Engine: a, Tick: t, Body: body})
}

// EndTick does nothing: a MemLog has already kept every record.
func (j *MemLog) EndTick(action.Address, uint64) {}
