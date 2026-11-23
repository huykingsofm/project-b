package game_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// cancelHeal is an action that cancels the heal that waits. Its ID is not one of the register:
// it is never written.
type cancelHeal struct{ priority int16 }

func (a *cancelHeal) ID() sim.ActionID { return 60010 }
func (a *cancelHeal) Priority() int16  { return a.priority }
func (a *cancelHeal) Actor() any       { return nil }
func (a *cancelHeal) Apply(
	_ *sim.Context, _ *zonelayerstate.State, r *sim.Registry[*zonelayerstate.State],
	_ *sim.Plan[*zonelayerstate.State],
) {
	r.CancelWaiting(sim.ActionKey{ID: action.ZoneLayerCharacterRegenActionID})
}

// A cancel reaches what waits and what was set before it; it does not reach an action that is in
// the plan of its tick, nor what that action sets after the cancel. So whether an action that sets
// itself again stops depends on the order of the plan, and the action decides for itself.
func TestACancelReachesWhatWaitsAndNotWhatAnActionInThePlanSetsAfterIt(t *testing.T) {
	heal := sim.ActionKey{ID: action.ZoneLayerCharacterRegenActionID}
	for _, tc := range []struct {
		name      string
		priority  int16
		tick      uint64
		stillWait bool
	}{
		{"the cancel runs while the heal waits (tick 4)", 0, 4, false},
		{"the cancel runs after the heal in the tick the heal is due (tick 10)", 0, 10, false},
		{"the cancel runs before the heal in the tick the heal is due (tick 10)", -300, 10, true},
	} {
		l := aWorldToCheckpoint(t).ZoneLayer(theLayer) // heals every 10 ticks
		l.ScheduleAt(tc.tick, &cancelHeal{priority: tc.priority})
		for range 30 {
			l.Step()
		}
		if _, waits := l.Waiting(heal); waits != tc.stillWait {
			t.Errorf("%s: the heal waits after 30 ticks: %v, want %v", tc.name, waits, tc.stillWait)
		}
	}
}
