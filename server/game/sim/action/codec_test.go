package action_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/server/game/sim/action"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonestate "github.com/huykingsofm/project-b/server/game/sim/state/zone"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/game/sim/tilemap"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/errs"
)

// everyAction has one action of every ID of the register, with every field set to something that is
// not the zero value, private fields included: an action that waits in a scheduler is partly run.
func everyAction() map[sim.ActionID]sim.BaseAction {
	ch, err := zonelayerstate.NewCharacter(7, "Binh", 100, 250, 256)
	if err != nil {
		panic(err)
	}
	ch.SetHealth(80)
	ch.SetCooldown(action.ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 44)
	at := state.Vec{X: -300, Y: 9000}
	area := tilemap.AreaOf(tilemap.Tile{X: 3, Y: 0}, tilemap.Tile{X: 3, Y: 2})
	tiles, err := tilemap.New(4, 3, tilemap.Tile{X: 1, Y: 1}, []tilemap.Tile{{X: 2, Y: 0}})
	if err != nil {
		panic(err)
	}
	normalAttack := action.ZoneLayerCharacterNormalAttackTriggerAction{CharacterID: 3, TargetID: 4}
	return map[sim.ActionID]sim.BaseAction{
		action.WorldChatPrivateSendActionID: action.WorldChatPrivateSendAction{FromID: 1, ToID: 2,
			Text: "psst"},
		action.WorldCharacterItemAddActionID: action.WorldCharacterItemAddAction{
			CharacterID: 3, ItemID: 8, Count: 5, FromZoneLayerID: 4, At: at, Expiry: 90,
		},
		action.ZoneChatSendActionID: action.ZoneChatSendAction{FromID: 3,
			Text: "xin chào"},
		action.ZoneCharacterEnterActionID: action.ZoneCharacterEnterAction{Character: ch},
		action.ZoneLayerChatSendActionID: action.ZoneLayerChatSendAction{FromID: 3,
			Text: "hello"},
		action.ZoneLayerCharacterEnterActionID: action.ZoneLayerCharacterEnterAction{Character: ch,
			At: at},
		action.ZoneLayerCharacterMoveActionID: action.ZoneLayerCharacterMoveAction{
			CharacterID: 3, Dir: state.East, Ticks: 6,
		},
		action.ZoneLayerCharacterNormalAttackTriggerActionID: normalAttack,
		action.ZoneLayerCharacterDamageActionID: action.ZoneLayerCharacterDamageAction{
			SourceID: anEntityID(1), VictimID: anEntityID(2), Amount: -5,
		},
		action.ZoneLayerCharacterShieldGrantActionID: action.ZoneLayerCharacterShieldGrantAction{
			CharacterID: 3, Duration: 20,
		},
		action.ZoneLayerCharacterRegenActionID: action.ZoneLayerCharacterRegenAction{},
		action.ZoneLayerCharacterExitToActionID: action.ZoneLayerCharacterExitToAction{
			CharacterID: 3, ExitEntityID: anEntityID(9),
		},
		action.ZoneLayerCharacterItemPickupActionID: action.ZoneLayerCharacterItemPickupAction{
			CharacterID: 3, ItemEntityID: anEntityID(12),
		},
		action.ZoneLayerItemEntityAddActionID: action.ZoneLayerItemEntityAddAction{
			ItemID: 8, Count: 5, At: at, Expiry: 40,
		},
		action.ZoneLayerItemEntityRemoveActionID: action.ZoneLayerItemEntityRemoveAction{
			EntityID: anEntityID(12),
		},
		action.WorldZoneAddActionID: action.WorldZoneAddAction{Tiles: tiles},
		action.WorldZoneLayerAddActionID: action.WorldZoneLayerAddAction{
			ZoneID: 3, Capacity: 40, RegenEvery: 10,
		},
		action.ZoneLayerAnnounceToZoneActionID: action.ZoneLayerAnnounceToZoneAction{Capacity: 40},
		action.ZoneZoneLayerAddActionID: action.ZoneZoneLayerAddAction{ZoneLayerID: 8,
			Capacity: 40},
		action.ZoneExitAddActionID: action.ZoneExitAddAction{
			Area: area, ToZoneID: 5, Arrival: state.Vec{X: 3, Y: -4},
		},
		action.WorldZoneExitAddActionID: action.WorldZoneExitAddAction{
			FromZoneID: 6, Area: area, ToZoneID: 5, Arrival: state.Vec{X: 3, Y: -4},
		},
	}
}

// anEntityID is the ID of the entity that a zonelayer gave the number local.
func anEntityID(local uint64) state.EntityID {
	return state.EntityID(local)
}

func codecs(t testing.TB) *sim.Codecs {
	t.Helper()
	c := sim.NewCodecs()
	if err := errs.Join(onfail.Register(c), action.Register(c)); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryActionOfTheRegisterHasACodecAndComesBackAsItWas(t *testing.T) {
	actions, c := everyAction(), codecs(t)
	for id := sim.ActionID(1); id <= action.WorldZoneExitAddActionID; id++ { // the last ID
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
		var w binary.Writer
		if err := sim.EncodeAction(&w, a); err != nil {
			t.Fatalf("%T: %v", a, err)
		}
		r := binary.NewReader(w.B)
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
		var w binary.Writer
		if err := sim.EncodeAction(&w, a); err != nil {
			t.Fatal(err)
		}
		decoded, err := c.DecodeAction(binary.NewReader(w.B), 1)
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

// The IDs that the messages of these tests go to.
var (
	worldID = sim.NewStateID(state.WorldType, 1)
	zoneID  = sim.NewStateID(state.ZoneType, 1)
	layerID = sim.NewStateID(state.ZoneLayerType, 1)
)

// aMessageOfEveryKind returns messages that between them use every kind of OnFail, a message
// inside a message, and each type of state.
func aMessageOfEveryKind() []sim.Message {
	actions := everyAction()
	layerActions := func() []sim.Action[*zonelayerstate.State] {
		return []sim.Action[*zonelayerstate.State]{
			actions[action.ZoneLayerCharacterMoveActionID].(sim.Action[*zonelayerstate.State]),
			actions[action.ZoneLayerChatSendActionID].(sim.Action[*zonelayerstate.State]),
		}
	}
	layer := func(onFail sim.OnFail) sim.Message {
		return sim.NewActionMessage(layerID, onFail, layerActions()...).
			Stamp(sim.Header{From: 2, Seq: 77}).Again()
	}
	undo := sim.NewActionMessage(
		layerID, onfail.Drop{},
		actions[action.ZoneLayerItemEntityAddActionID].(sim.Action[*zonelayerstate.State]),
	)
	return []sim.Message{
		layer(onfail.Retry{}),
		layer(onfail.Drop{}),
		layer(onfail.Redirect{To: layerID + 1}),
		layer(onfail.Escalate{To: zoneID}),
		layer(onfail.Compensate{With: undo}),
		layer(onfail.Compensate{With: sim.NewActionMessage[*zonestate.State](
			zoneID, onfail.Compensate{With: undo},
		)}),
		sim.NewActionMessage(
			zoneID, onfail.Drop{},
			actions[action.ZoneChatSendActionID].(sim.Action[*zonestate.State]),
		),
		sim.NewActionMessage(
			worldID, onfail.Drop{},
			actions[action.WorldChatPrivateSendActionID].(sim.Action[*worldstate.State]),
		),
		sim.NewActionMessage[*worldstate.State](worldID, onfail.Drop{}),
	}
}

func TestAMessageOfEveryKindComesBackWholeAndCutBytesAreAnErrorNeverAPanic(t *testing.T) {
	c := codecs(t)
	registerStates(t, c)
	for i, m := range aMessageOfEveryKind() {
		var w binary.Writer
		if err := sim.EncodeMessage(&w, m); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		r := binary.NewReader(w.B)
		got, err := c.DecodeMessage(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("message %d: error %v, %d bytes left", i, err, r.Left())
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("message %d came back as %+v, want %+v", i, got, m)
		}
		for n := range w.B { // every proper prefix
			if _, err := c.DecodeMessage(binary.NewReader(w.B[:n]), 1); err == nil {
				t.Fatalf("message %d: %d of %d bytes were read", i, n, len(w.B))
			}
		}
		for at := range w.B { // every byte made something else
			bad := append([]byte(nil), w.B...)
			bad[at] ^= 0xff
			c.DecodeMessage(binary.NewReader(bad), 1) // an error or a message, no panic
		}
	}
}

// decodeZoneWithNoMap reads nothing: these tests write messages, and no state.
func decodeZoneWithNoMap(*binary.Reader, codec.Version) (*zonestate.State, error) {
	return nil, nil
}

// registerStates registers the states, which a message needs to be read back: it says the type of
// state its actions are for.
func registerStates(t testing.TB, c *sim.Codecs) {
	t.Helper()
	for _, register := range []error{
		c.RegisterState(worldstate.DecodeState),
		c.RegisterState(decodeZoneWithNoMap),
		c.RegisterState(zonelayerstate.DecodeState),
	} {
		if register != nil {
			t.Fatal(register)
		}
	}
}

// A plan finds the actions of an actor by a column keyed by the actor, so what Actor returns can be
// compared with ==.
func TestTheActorOfEveryActionThatHasOneCanBeCompared(t *testing.T) {
	n := 0
	for id, a := range everyAction() {
		actor := a.Actor()
		if actor == nil {
			continue
		}
		n++
		if !reflect.TypeOf(actor).Comparable() {
			t.Errorf("the actor of the action %d (%T) is a %T: it cannot be compared", id, a, actor)
		}
	}
	if n == 0 {
		t.Fatal("no action of the game has an actor: the test checks nothing")
	}
}
