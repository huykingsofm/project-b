package action

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// everyAction has one action of every ID of the register, with every field set to something that is
// not the zero value, private fields included: an action that waits in a scheduler is partly run.
func everyAction() map[sim.ActionID]sim.BaseAction {
	ch, err := zonelayerstate.NewCharacter(7, "Binh", 100, 250, 256)
	if err != nil {
		panic(err)
	}
	ch.SetHealth(80)
	ch.SetCooldown(ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 44)
	at := state.Vec{X: -300, Y: 9000}
	return map[sim.ActionID]sim.BaseAction{
		WorldChatPrivateSendActionID: &WorldChatPrivateSendAction{FromID: 1, ToID: 2, Text: "psst"},
		WorldCharacterItemAddActionID: &WorldCharacterItemAddAction{
			CharacterID: 3, ItemID: 8, Count: 5, FromZoneLayerID: 4, At: at, Expiry: 90,
		},
		ZoneChatSendActionID:            &ZoneChatSendAction{FromID: 3, Text: "xin chào"},
		ZoneCharacterEnterActionID:      &ZoneCharacterEnterAction{Character: ch},
		ZoneLayerChatSendActionID:       &ZoneLayerChatSendAction{FromID: 3, Text: "hello"},
		ZoneLayerCharacterEnterActionID: &ZoneLayerCharacterEnterAction{Character: ch, At: at},
		ZoneLayerCharacterMoveActionID: &ZoneLayerCharacterMoveAction{
			CharacterID: 3, Dir: state.East, Ticks: 6, reserved: true,
		},
		ZoneLayerCharacterNormalAttackTriggerActionID: &ZoneLayerCharacterNormalAttackTriggerAction{
			CharacterID: 3, TargetID: 4,
		},
		ZoneLayerCharacterDamageActionID: &ZoneLayerCharacterDamageAction{
			SourceID: 1, VictimID: 2, Amount: -5,
		},
		ZoneLayerCharacterShieldGrantActionID: &ZoneLayerCharacterShieldGrantAction{
			CharacterID: 3, Duration: 20, started: true, until: 31,
		},
		ZoneLayerCharacterRegenActionID: &ZoneLayerCharacterRegenAction{},
		ZoneLayerCharacterExitToActionID: &ZoneLayerCharacterExitToAction{
			CharacterID: 3, ExitEntityID: 9,
		},
		ZoneLayerCharacterItemPickupActionID: &ZoneLayerCharacterItemPickupAction{
			CharacterID: 3, ItemEntityID: 12,
		},
		ZoneLayerItemEntityAddActionID: &ZoneLayerItemEntityAddAction{
			ItemID: 8, Count: 5, At: at, Expiry: 40,
		},
		ZoneLayerItemEntityRemoveActionID: &ZoneLayerItemEntityRemoveAction{EntityID: 12},
	}
}

func codecs(t testing.TB) *sim.Codecs {
	t.Helper()
	c := sim.NewCodecs()
	if err := sim.Errors(onfail.Register(c), Register(c)); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryActionOfTheRegisterHasACodecAndComesBackAsItWas(t *testing.T) {
	actions, c := everyAction(), codecs(t)
	for id := sim.ActionID(1); id <= ZoneLayerItemEntityRemoveActionID; id++ { // the last ID
		a, ok := actions[id]
		if !ok {
			t.Fatalf("the action with the ID %d has no sample in this test", id)
		}
		if a.ID() != id {
			t.Fatalf("the sample for %d is a %T, which has the ID %d", id, a, a.ID())
		}
		if !c.Actions.Has(id) {
			t.Fatalf("%T has no decoder", a)
		}
		var w binarycodec.Writer
		if err := sim.EncodeAction(&w, a); err != nil {
			t.Fatalf("%T: %v", a, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := c.DecodeAction(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("%T: error %v, %d bytes left", a, err, r.Left())
		}
		if !reflect.DeepEqual(got, a) {
			t.Fatalf("%T came back as %+v, want %+v", a, got, a)
		}
	}
}

func TestEveryActionOfThePackageIsRegisteredForItsOwnStateType(t *testing.T) {
	// an action of a zonelayer can be sent to a zonelayer and to no other state: the types the
	// compiler checked when it was registered
	c := codecs(t)
	for id, a := range everyAction() {
		var w binarycodec.Writer
		if err := sim.EncodeAction(&w, a); err != nil {
			t.Fatal(err)
		}
		decoded, err := c.DecodeAction(binarycodec.NewReader(w.B), 1)
		if err != nil {
			t.Fatal(err)
		}
		_, world := decoded.(sim.Action[*worldstate.State])
		_, zone := decoded.(sim.Action[*zonestate.State])
		_, layer := decoded.(sim.Action[*zonelayerstate.State])
		n := 0
		for _, is := range []bool{world, zone, layer} {
			if is {
				n++
			}
		}
		if n != 1 {
			t.Errorf("the action %d (%T) is an action of %v, %v and %v", id, a, world, zone, layer)
		}
	}
}

// aMessageOfEveryKind returns messages that between them use every kind of OnFail, a message
// inside a message, and each type of state.
func aMessageOfEveryKind() []sim.Message {
	actions := everyAction()
	layerActions := func() []sim.Action[*zonelayerstate.State] {
		return []sim.Action[*zonelayerstate.State]{
			actions[ZoneLayerCharacterMoveActionID].(sim.Action[*zonelayerstate.State]),
			actions[ZoneLayerChatSendActionID].(sim.Action[*zonelayerstate.State]),
		}
	}
	layer := func(onFail sim.OnFail) sim.Message {
		return sim.NewActionMessage(5, onFail, layerActions()...).
			Stamp(sim.Header{From: 2, Seq: 77}).Again()
	}
	undo := sim.NewActionMessage(
		5, onfail.Drop{},
		actions[ZoneLayerItemEntityAddActionID].(sim.Action[*zonelayerstate.State]),
	)
	return []sim.Message{
		layer(onfail.Retry{}),
		layer(onfail.Drop{}),
		layer(onfail.Redirect{To: 6}),
		layer(onfail.Escalate{To: 1}),
		layer(onfail.Notify{Key: "item.gone", Args: []string{"sword", "3"}}),
		layer(onfail.Notify{Key: "item.gone"}),
		layer(onfail.Compensate{With: undo}),
		layer(onfail.Compensate{With: sim.NewActionMessage[*zonestate.State](
			1, onfail.Compensate{With: undo},
		)}),
		sim.NewActionMessage(
			1, onfail.Drop{}, actions[ZoneChatSendActionID].(sim.Action[*zonestate.State]),
		),
		sim.NewActionMessage(
			1, onfail.Drop{},
			actions[WorldChatPrivateSendActionID].(sim.Action[*worldstate.State]),
		),
		sim.NewActionMessage[*worldstate.State](1, onfail.Drop{}),
	}
}

func TestAMessageOfEveryKindComesBackWholeAndCutBytesAreAnErrorNeverAPanic(t *testing.T) {
	c := codecs(t)
	registerStates(t, c)
	for i, m := range aMessageOfEveryKind() {
		var w binarycodec.Writer
		if err := sim.EncodeMessage(&w, m); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := c.DecodeMessage(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("message %d: error %v, %d bytes left", i, err, r.Left())
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("message %d came back as %+v, want %+v", i, got, m)
		}
		for n := range w.B { // every proper prefix
			if _, err := c.DecodeMessage(binarycodec.NewReader(w.B[:n]), 1); err == nil {
				t.Fatalf("message %d: %d of %d bytes were read", i, n, len(w.B))
			}
		}
		for at := range w.B { // every byte made something else
			bad := append([]byte(nil), w.B...)
			bad[at] ^= 0xff
			c.DecodeMessage(binarycodec.NewReader(bad), 1) // an error or a message, no panic
		}
	}
}

// decodeZoneWithNoMap reads nothing: these tests write messages, and no state.
func decodeZoneWithNoMap(*binarycodec.Reader, binarycodec.Version) (*zonestate.State, error) {
	return nil, nil
}

// registerStates registers the states, which a message needs to be read back: it says the type of
// state its actions are for.
func registerStates(t testing.TB, c *sim.Codecs) {
	t.Helper()
	for _, register := range []error{
		c.RegisterState[*worldstate.State](&worldstate.State{}, worldstate.DecodeState),
		c.RegisterState[*zonestate.State](&zonestate.State{}, decodeZoneWithNoMap),
		c.RegisterState[*zonelayerstate.State](&zonelayerstate.State{}, zonelayerstate.DecodeState),
	} {
		if register != nil {
			t.Fatal(register)
		}
	}
}
