package action

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

func benchMessages() map[string]message.Message {
	actions := everyAction()
	two := MessageToZoneLayer(5, onfail.Drop{},
		actions[ZoneLayerCharacterMoveActionID].(ZoneLayerAction),
		actions[ZoneLayerChatSendActionID].(ZoneLayerAction),
	).Stamp(message.Header{From: message.ZoneAddress(2), Seq: 98765})
	undo := MessageToZoneLayer(
		5, onfail.Drop{}, actions[ZoneLayerItemEntityAddActionID].(ZoneLayerAction),
	)
	return map[string]message.Message{
		"2 actions": two,
		"enter, a character": MessageToZoneLayer(
			5, onfail.Drop{}, actions[ZoneLayerCharacterEnterActionID].(ZoneLayerAction),
		),
		"pickup, with compensator": MessageToWorld(
			1, onfail.Compensate{With: undo}, actions[WorldCharacterItemAddActionID].(WorldAction),
		),
	}
}

func BenchmarkEncodeMessage(b *testing.B) {
	for name, m := range benchMessages() {
		b.Run(name, func(b *testing.B) {
			var w binarycodec.Writer
			b.ReportAllocs()
			for b.Loop() {
				w.B = w.B[:0]
				if err := EncodeMessage(&w, m); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecodeMessage(b *testing.B) {
	for name, m := range benchMessages() {
		var w binarycodec.Writer
		if err := EncodeMessage(&w, m); err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := DecodeMessage(binarycodec.NewReader(w.B), 1); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
