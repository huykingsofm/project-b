package enginetest

import (
	"testing"

	"github.com/huykingsofm/project-b/server/internal/config"
	"github.com/huykingsofm/project-b/server/internal/services/game/engine"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Sender gives the commands of a scenario to the engines as they are, or after they were written to
// bytes and read back: a copy that shares nothing with what the caller made, as a command that came
// from a client does.
type Sender struct {
	T         testing.TB
	RoundTrip bool
	Reuses    bool // the Sender goes on using the value it sent, and changes it
}

func Via[C command.Command](s Sender, c C) C {
	if !s.RoundTrip {
		return c
	}
	var w binarycodec.Writer
	if err := command.Encode(&w, c); err != nil {
		s.T.Fatal(err)
	}
	got, err := command.Decode(binarycodec.NewReader(w.B), 1)
	if err != nil {
		s.T.Fatal(err)
	}
	return got.(C)
}

// RunScenario runs a cluster through chat in each scope, a shield, an attack, a pickup, a run of
// movement, a new version of the config, a character that enters with cooldowns and one that
// crosses to another zone, long enough for every engine to record a hash.
func RunScenario(t testing.TB, c *engine.Cluster, cfg *config.Store, s Sender) {
	t.Helper()
	an := NewCharacter(1, "An", 100, 1000, 0)
	an.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 9)
	c.Zone(1).Send(Via(s, command.ZoneCharacterEnterCommand{Character: an}))
	if s.Reuses {
		an.Cooldowns.Set(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 1<<40)
	}
	Steps(c, 4)
	c.Zone(1).Send(
		Via(s, command.ZoneCharacterEnterCommand{Character: NewCharacter(2, "Binh", 100, 1000, 0)}),
	)
	Steps(c, 4)
	// The item reaches the ground the way a message puts it there, which the log records; a change
	// made on the state from outside the engine would be in no log, and no replay could make it.
	c.Env.Router.Deliver(action.MessageToZoneLayer(
		1, onfail.Drop{},
		&action.ZoneLayerItemEntityAddAction{ItemID: 7, Count: 1, At: state.Vec{X: 2500, Y: 2500}},
	).Stamp(message.Header{From: message.ZoneAddress(99), Seq: 1}))
	Steps(c, 1)
	var itemID state.EntityID
	for _, id := range c.ZoneLayer(1).State().Entities.Keys() {
		if _, ok := c.ZoneLayer(1).State().ItemEntityByID(id); ok {
			itemID = id
		}
	}

	l := c.ZoneLayer(1)
	c.World.Send(Via(s, command.WorldChatPrivateSendCommand{FromID: 10, ToID: 20, Text: "psst"}))
	c.Zone(1).Send(Via(s, command.ZoneChatSendCommand{FromID: 1, Text: "hello zone"}))
	l.Send(Via(s, command.ZoneLayerChatSendCommand{FromID: 1, Text: "xin chào"}))
	l.Send(Via(s, command.ZoneLayerCharacterShieldGrantCommand{CharacterID: 2, Duration: 100}))
	l.Send(
		Via(s, command.ZoneLayerCharacterNormalAttackTriggerCommand{CharacterID: 1, TargetID: 2}),
	)
	l.Send(
		Via(s, command.ZoneLayerCharacterItemPickupCommand{CharacterID: 1, ItemEntityID: itemID}),
	)
	Steps(c, 6)
	if _, err := cfg.Apply(
		map[string]string{"zonelayer.normal_attack.cooldown_ticks": "5"},
	); err != nil { // a new version of the config: the engines record that they read it
		t.Fatal(err)
	}
	l.Send(Via(s, command.ZoneLayerCharacterMoveCommand{CharacterID: 1, Dir: state.East, Ticks: 8}))
	Steps(c, 14)
	l.Send(Via(s, command.ZoneLayerCharacterExitToCommand{CharacterID: 1, ExitEntityID: 1}))
	Steps(c, 60) // the zone it went to records a hash too
}
