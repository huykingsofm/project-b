package engine_test

import (
	"context"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine/enginetest"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
)

// A caller that keeps the character it sent (a gateway, an adapter that saves it) and reads it
// while the engine runs must not race with the engine: the state that the character enters owns its
// own copy of it (R29).
func TestACallerThatKeepsTheCharacterItSentCanReadItWhileTheEngineChangesItsCooldowns(
	t *testing.T,
) {
	c, _, _, _ := enginetest.NewWorld(t, 1, 0)
	kept := enginetest.NewCharacter(1, "An", 100, 1000, 0)
	kept.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 5)
	kept.Cooldowns.Set(action.ZoneLayerCharacterMoveActionCooldownKey, 7)
	target := enginetest.NewCharacter(2, "Binh", 1000, 1000, 0)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { c.Run(ctx, time.Millisecond) })

	c.Zone(1).Send(command.ZoneCharacterEnterCommand{Character: kept})
	c.Zone(1).Send(command.ZoneCharacterEnterCommand{Character: target})
	time.Sleep(20 * time.Millisecond)

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		c.ZoneLayer(1).Send(
			command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2},
		)
		c.ZoneLayer(1).Send(
			command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 1},
		)
		// the caller reads what it kept, as a gateway that saves the character would
		_ = kept.Cooldowns.Len()
		for range kept.Cooldowns.All() {
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	wg.Wait()

	if got, _ := kept.Cooldowns.Get(
		action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey,
	); got != 5 {
		t.Fatalf("the engine changed the cooldown of the character that was sent: %d, want 5", got)
	}
	if kept.Cooldowns.Len() != 2 {
		t.Fatalf(
			"the engine changed the cooldowns of the character that was sent: %d of them, want 2",
			kept.Cooldowns.Len(),
		)
	}
}
