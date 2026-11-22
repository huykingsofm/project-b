package engine

import (
	"sync"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
)

// PlanPool lends plans to the engines of one scope. A plan lives for one tick, so the plans in use
// at a moment are about as many as the ticks that run at that moment, not as many as the engines;
// the pool keeps the memory of a plan (its slices and its map) for the next tick of any engine of
// the scope. It is safe for many goroutines. It is in the engine package and not in sim because it
// is not data: it is the running of the engines that needs it.
type PlanPool[A action.Action] struct{ pool sync.Pool }

// Get returns an empty plan that reads the configuration of the tick.
func (p *PlanPool[A]) Get(cfg *config.Snapshot) *action.Plan[A] {
	plan, ok := p.pool.Get().(*action.Plan[A])
	if !ok {
		plan = action.NewPlan[A]()
	}
	plan.Reset(cfg)
	return plan
}

// Put empties the plan, so that it holds no action and no configuration, and keeps it for the next
// Get. The plan must not be used after it is put back.
func (p *PlanPool[A]) Put(plan *action.Plan[A]) {
	plan.Reset(nil)
	p.pool.Put(plan)
}

// The pools of the three scopes: every world engine, zone engine and zonelayer engine takes its
// plan for a tick from the one of its scope.
var (
	worldPlans     PlanPool[action.WorldAction]
	zonePlans      PlanPool[action.ZoneAction]
	zoneLayerPlans PlanPool[action.ZoneLayerAction]
)
