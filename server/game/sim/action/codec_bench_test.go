package action

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	worldstate "github.com/huykingsofm/project-b/server/game/sim/state/world"
	zonelayerstate "github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

func benchMessages() map[string]sim.Message {
	actions := everyAction()
	layer := func(id sim.ActionID) sim.Action[*zonelayerstate.State] {
		return actions[id].(sim.Action[*zonelayerstate.State])
	}
	two := sim.NewActionMessage(
		layerID, onfail.Drop{},
		layer(ZoneLayerCharacterMoveActionID), layer(ZoneLayerChatSendActionID),
	).Stamp(sim.Header{From: 2, Seq: 98765})
	undo := sim.NewActionMessage(layerID, onfail.Drop{}, layer(ZoneLayerItemEntityAddActionID))
	return map[string]sim.Message{
		"2 actions": two,
		"enter, a character": sim.NewActionMessage(
			layerID, onfail.Drop{}, layer(ZoneLayerCharacterEnterActionID),
		),
		"pickup, with compensator": sim.NewActionMessage(
			worldID, onfail.Compensate{With: undo},
			actions[WorldCharacterItemAddActionID].(sim.Action[*worldstate.State]),
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
				if err := sim.EncodeMessage(&w, m); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecodeMessage(b *testing.B) {
	c := codecs(b)
	registerStates(b, c)
	for name, m := range benchMessages() {
		var w binarycodec.Writer
		if err := sim.EncodeMessage(&w, m); err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := c.DecodeMessage(binarycodec.NewReader(w.B), 1); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
