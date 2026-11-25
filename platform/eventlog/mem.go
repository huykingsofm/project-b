package eventlog

import (
	"sync"

	"github.com/huykingsofm/project-b/platform/sim"
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

// Append keeps a record.
func (j *MemLog) Append(a sim.StateID, t uint64, body Body) {
	j.add(Record{Engine: a, Tick: t, Body: body})
}

// EndTick does nothing: a MemLog has already kept every record.
func (j *MemLog) EndTick(sim.StateID, uint64) {}

// Safe says every tick is: a MemLog is for tests, which have no crash to lose a tick to.
func (j *MemLog) Safe(sim.StateID) uint64 { return ^uint64(0) }
