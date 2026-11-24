package action

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/testing/must"
)

func TestRegenHealsEveryoneAndSetsItselfAgainEveryPeriodUnlessThePeriodIsZero(t *testing.T) {
	st := zonelayerstate.NewState(1, 1, 9, 1000)
	c, err := zonelayerstate.NewCharacter(1, "An", 100, 250, 256)
	if err != nil {
		t.Fatal(err)
	}
	body := must.Ok(st.AddCharacterEntity(c, state.Vec{X: 2048, Y: 2048}))
	st.DamageEntity(body, 30)
	health := func() int32 { ch, _ := st.CharacterByID(1); return ch.Health() }

	var reg sim.Registry[*zonelayerstate.State]
	reg.Begin(10)
	a := &ZoneLayerCharacterRegenAction{Every: 7}
	a.Apply(&sim.Context{Tick: 10}, st, &reg, sim.NewPlan[*zonelayerstate.State]())
	if health() != 71 {
		t.Fatalf("health %d after the heal, want 71", health())
	}
	later := reg.TakeSelf()
	if len(later) != 1 || later[0].At != 17 ||
		later[0].Action != sim.Action[*zonelayerstate.State](a) {
		t.Fatalf("it set %v, want itself at 17", later)
	}

	once := &ZoneLayerCharacterRegenAction{}
	once.Apply(&sim.Context{Tick: 17}, st, &reg, sim.NewPlan[*zonelayerstate.State]())
	if later := reg.TakeSelf(); health() != 72 || len(later) != 0 {
		t.Fatalf("a period of 0 healed to %d and set %d again", health(), len(later))
	}
}
