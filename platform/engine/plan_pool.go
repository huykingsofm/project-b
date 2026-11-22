package engine

import (
	"reflect"
	"sync"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
)

// PlanPool lends plans to the engines of one type of state. A plan lives for one tick, so the plans
// in use at a moment are about as many as the ticks that run at that moment, not as many as the
// engines; the pool keeps the memory of a plan (its slices and its map) for the next tick of any
// engine of the type. It is safe for many goroutines. It is in the engine package and not in sim
// because it is not data: it is the running of the engines that needs it.
type PlanPool[S sim.State] struct{ pool sync.Pool }

// Get returns an empty plan that reads the configuration of the tick.
func (p *PlanPool[S]) Get(cfg *config.Snapshot) *sim.Plan[S] {
	plan, ok := p.pool.Get().(*sim.Plan[S])
	if !ok {
		plan = sim.NewPlan[S]()
	}
	plan.Reset(cfg)
	return plan
}

// Put empties the plan, so that it holds no action and no configuration, and keeps it for the next
// Get. The plan must not be used after it is put back.
func (p *PlanPool[S]) Put(plan *sim.Plan[S]) {
	plan.Reset(nil)
	p.pool.Put(plan)
}

// planPools holds one PlanPool for each Go type of state: every engine of a type takes its plan for
// a tick from the pool of its type.
var planPools sync.Map // reflect.Type -> *PlanPool[S]

// planPoolOf returns the pool of plans for the engines whose state is an S.
func planPoolOf[S sim.State]() *PlanPool[S] {
	key := reflect.TypeFor[S]()
	if p, ok := planPools.Load(key); ok {
		return p.(*PlanPool[S])
	}
	p, _ := planPools.LoadOrStore(key, new(PlanPool[S]))
	return p.(*PlanPool[S])
}
