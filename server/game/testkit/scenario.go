package testkit

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/command"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Sender gives the commands of a scenario to the engines as they are, or after they were written to
// bytes and read back: a copy that shares nothing with what the caller made, as a command that came
// from a client does.
type Sender struct {
	T         testing.TB
	RoundTrip bool
	Codecs    *sim.Codecs // reads the commands back, when RoundTrip is set
	Reuses    bool        // the Sender goes on using the value it sent, and changes it
}

func (s Sender) Via[C sim.BaseCommand](c C) C {
	if !s.RoundTrip {
		return c
	}
	var w binary.Writer
	if err := sim.EncodeCommand(&w, c); err != nil {
		s.T.Fatal(err)
	}
	got, err := s.Codecs.DecodeCommand(binary.NewReader(w.B), 1)
	if err != nil {
		s.T.Fatal(err)
	}
	return got.(C)
}

// RunScenario runs a cluster through chat in each scope, a shield, an attack, a pickup, a run of
// movement, a new version of the config, a character that enters with cooldowns and one that
// crosses to another zone, long enough for every engine to record a hash.
func RunScenario(t testing.TB, c *game.Cluster, cfg *config.Store, s Sender) {
	t.Helper()
	an := NewCharacter(1, "An", 100, 1000, 0)
	an.SetCooldown(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 9)
	c.Zone(Zone1).Send(s.Via(command.ZoneCharacterEnterCommand{Character: an}))
	if s.Reuses {
		an.SetCooldown(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 1<<40)
	}
	Steps(t, c, 4)
	c.Zone(Zone1).Send(
		s.Via(command.ZoneCharacterEnterCommand{Character: NewCharacter(2, "Binh", 100, 1000, 0)}),
	)
	Steps(t, c, 4)
	// The item reaches the ground the way a message puts it there, which the log records; a change
	// made on the state from outside the engine would be in no log, and no replay could make it.
	c.Env.Router.Deliver(sim.NewActionMessage[*zonelayerstate.State](
		Layer1, onfail.Drop{},
		action.ZoneLayerItemEntityAddAction{ItemID: 7, Count: 1, At: state.Vec{X: 2500, Y: 2500}},
	).Stamp(sim.Header{From: 99, Seq: 1}))
	Steps(t, c, 1)
	var itemID state.EntityID
	for _, id := range c.ZoneLayer(Layer1).State().EntityIDsOfKind(state.ItemKindID) {
		itemID = id
	}

	l := c.ZoneLayer(Layer1)
	c.World.Send(s.Via(command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"}))
	c.Zone(Zone1).Send(s.Via(command.ZoneChatSendCommand{FromID: 1, Text: "hello zone"}))
	l.Send(s.Via(command.ZoneLayerChatSendCommand{FromID: 1, Text: "xin chào"}))
	l.Send(s.Via(command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100}))
	l.Send(
		s.Via(command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2}),
	)
	l.Send(
		s.Via(command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID}),
	)
	Steps(t, c, 6)
	if _, err := cfg.Apply(
		map[string]string{"zonelayer.normal_attack.cooldown_ticks": "5"},
	); err != nil { // a new version of the config: the engines record that they read it
		t.Fatal(err)
	}
	l.Send(s.Via(command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8}))
	Steps(t, c, 14)
	l.Send(s.Via(command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: EastExit}))
	Steps(t, c, 60) // the zone it went to records a hash too
}
