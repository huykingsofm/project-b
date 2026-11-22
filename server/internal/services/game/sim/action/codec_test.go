package action

import (
	"errors"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/state"
	zonelayerstate "github.com/huykingsofm/project-b/server/internal/services/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// everyAction has one action of every ID of the register, with every field set to something that is
// not the zero value, private fields included: an action that waits in a scheduler is partly run.
func everyAction() map[ID]Action {
	ch := zonelayerstate.Character{
		CharacterID: 7, Name: "Binh", Health: 80, MaxHealth: 100, Radius: 256, Speed: 250,
	}
	ch.Cooldowns.Set(ZoneLayerCharacterNormalAttackTriggerActionCooldownKey, 44)
	at := state.Vec{X: -300, Y: 9000}
	return map[ID]Action{
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

func TestEveryActionOfTheRegisterHasACodecAndComesBackAsItWas(t *testing.T) {
	actions := everyAction()
	for id := ID(1); id <= ZoneLayerItemEntityRemoveActionID; id++ { // the last ID of the register
		a, ok := actions[id]
		if !ok {
			t.Fatalf("the action with the ID %d has no sample in this test", id)
		}
		if a.ID() != id {
			t.Fatalf("the sample for %d is a %T, which has the ID %d", id, a, a.ID())
		}
		if _, ok := actionDecoders[id]; !ok {
			t.Fatalf("%T has no decoder", a)
		}
		var w binarycodec.Writer
		if err := EncodeAction(&w, a); err != nil {
			t.Fatalf("%T: %v", a, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := DecodeAction(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("%T: error %v, %d bytes left", a, err, r.Left())
		}
		if !reflect.DeepEqual(got, a) {
			t.Fatalf("%T came back as %+v, want %+v", a, got, a)
		}
	}
}

func TestEveryScopeHasAMessageDecoder(t *testing.T) {
	scopes := []message.Scope{message.ScopeWorld, message.ScopeZone, message.ScopeZoneLayer}
	for _, s := range scopes {
		if _, ok := messageDecoders[s]; !ok {
			t.Errorf("the scope %v has no message decoder", s)
		}
	}
}

// aMessageOfEveryKind returns messages that between them use every kind of OnFail, a message
// inside a message, and each scope.
func aMessageOfEveryKind() []message.Message {
	actions := everyAction()
	layer := func(onFail message.OnFail) message.Message {
		return MessageToZoneLayer(5, onFail,
			actions[ZoneLayerCharacterMoveActionID].(ZoneLayerAction),
			actions[ZoneLayerChatSendActionID].(ZoneLayerAction),
		).Stamp(message.Header{From: message.ZoneAddress(2), Seq: 77}).Again()
	}
	undo := MessageToZoneLayer(
		5, onfail.Drop{}, actions[ZoneLayerItemEntityAddActionID].(ZoneLayerAction),
	)
	return []message.Message{
		layer(onfail.Retry{}),
		layer(onfail.Drop{}),
		layer(onfail.Redirect{To: message.ZoneLayerAddress(6)}),
		layer(onfail.Escalate{To: message.ZoneAddress(1)}),
		layer(onfail.Notify{Key: "item.gone", Args: []string{"sword", "3"}}),
		layer(onfail.Notify{Key: "item.gone"}),
		layer(onfail.Compensate{With: undo}),
		layer(onfail.Compensate{With: MessageToZone(1, onfail.Compensate{With: undo})}),
		MessageToZone(1, onfail.Drop{}, actions[ZoneChatSendActionID].(ZoneAction)),
		MessageToWorld(1, onfail.Drop{}, actions[WorldChatPrivateSendActionID].(WorldAction)),
		MessageToWorld(1, onfail.Drop{}),
	}
}

func TestAMessageComesBackWholeWithItsOnFailAndTheMessageInsideIt(t *testing.T) {
	for i, m := range aMessageOfEveryKind() {
		var w binarycodec.Writer
		if err := EncodeMessage(&w, m); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := DecodeMessage(r, 1)
		if err != nil || r.Left() != 0 {
			t.Fatalf("message %d: error %v, %d bytes left", i, err, r.Left())
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("message %d came back as %+v, want %+v", i, got, m)
		}
	}
}

func TestBytesThatAreCutOffOrWrongGiveAnErrorForAMessageAndNeverAPanic(t *testing.T) {
	for i, m := range aMessageOfEveryKind() {
		var w binarycodec.Writer
		if err := EncodeMessage(&w, m); err != nil {
			t.Fatal(err)
		}
		for n := range w.B { // every proper prefix
			if _, err := DecodeMessage(binarycodec.NewReader(w.B[:n]), 1); err == nil {
				t.Fatalf("message %d: %d of %d bytes were read", i, n, len(w.B))
			}
		}
		for at := range w.B { // every byte made something else
			bad := append([]byte(nil), w.B...)
			bad[at] ^= 0xff
			DecodeMessage(binarycodec.NewReader(bad), 1) // an error or a message, no panic
		}
	}
}

func TestAMessageWithAnActionOfAnotherScopeOrTooDeepIsRefused(t *testing.T) {
	var w binarycodec.Writer
	w.Uint8(uint8(message.ScopeWorld)) // the address says world
	w.Uvarint(1)
	w.Uint8(0)
	w.Uvarint(0)
	w.Uvarint(0) // the header: the sender and the number
	w.Uvarint(0) // attempts
	w.Uvarint(uint64(onfail.DropID))
	w.Uvarint(1)
	zoneAction := &ZoneChatSendAction{FromID: 1, Text: "x"}
	if err := EncodeAction(&w, zoneAction); err != nil {
		t.Fatal(err)
	}
	_, err := DecodeMessage(binarycodec.NewReader(w.B), 1)
	if !errors.Is(err, message.ErrUnknownCode) {
		t.Fatalf("an action of another scope: %v, want message.ErrUnknownCode", err)
	}

	deep := MessageToZone(1, onfail.Drop{})
	for range 9 {
		deep = MessageToZone(1, onfail.Compensate{With: deep})
	}
	var d binarycodec.Writer
	if err := EncodeMessage(&d, deep); err != nil {
		t.Fatal(err)
	}
	_, err = DecodeMessage(binarycodec.NewReader(d.B), 1)
	if !errors.Is(err, binarycodec.ErrBadValue) {
		t.Fatalf("messages nested 9 deep: %v, want ErrBadValue", err)
	}
}

// noCodecAction is an action that cannot be written.
type noCodecAction struct{}

func (noCodecAction) ID() ID          { return ZoneLayerCharacterRegenActionID }
func (noCodecAction) Priority() int16 { return 0 }
func (noCodecAction) ApplyZoneLayer(
	*Context, *zonelayerstate.State, *ZoneLayerRegistry, *Plan[ZoneLayerAction],
) {
}

func TestAnActionWithNoEncodeAndAMessageThatHoldsOneAreNotWrittenAtAll(t *testing.T) {
	var w binarycodec.Writer
	err := EncodeAction(&w, noCodecAction{})
	if !errors.Is(err, message.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("an action with no Encode: %v, %d bytes written", err, len(w.B))
	}
	m := MessageToZoneLayer(
		1, onfail.Drop{}, &ZoneLayerCharacterRegenAction{}, noCodecAction{},
	)
	if err := EncodeMessage(&w, m); !errors.Is(err, message.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a message with such an action: %v, %d bytes written", err, len(w.B))
	}
	if err := EncodeMessage(&w, MessageToZone(1, nil)); !errors.Is(err, message.ErrNoCodec) {
		t.Fatalf("a message with no OnFail: %v, want message.ErrNoCodec", err)
	}
}
