package game

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// EnsureRegen makes the zonelayer heal its characters every every ticks, which one action does: it
// heals and sets itself again. When no such action waits, the first is set for every ticks from
// now; when one waits, it must be the one asked for. One that waits with another period is an
// error, because a period that changed while the cluster ran would change what a replay makes, and
// so is one that waits when the period is 0, which means no healing.
func EnsureRegen(e *engine.Engine[*zonelayerstate.State], every uint64) error {
	key := sim.ActionKey{ID: action.ZoneLayerCharacterRegenActionID}
	waiting, found := e.Waiting(key)
	if !found {
		if every > 0 {
			e.ScheduleAt(e.Tick()+every, &action.ZoneLayerCharacterRegenAction{Every: every})
		}
		return nil
	}
	regen, ok := waiting.(*action.ZoneLayerCharacterRegenAction)
	if !ok || regen.Every != every {
		return fmt.Errorf(
			"zonelayer %d: the zonelayer heals every %d ticks and the cluster says every %d",
			e.StateID(), regenEveryOf(waiting), every,
		)
	}
	return nil
}

// regenEveryOf is the period of an action that heals, or 0.
func regenEveryOf(a sim.Action[*zonelayerstate.State]) uint64 {
	if regen, ok := a.(*action.ZoneLayerCharacterRegenAction); ok {
		return regen.Every
	}
	return 0
}
