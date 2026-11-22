package replay

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"testing"
)

func TestEveryKindOfTheRegisterHasADecoderAndABodyThatReturnsIt(t *testing.T) {
	bodies := []Body{
		Inbound{Message: benchMessage(1)}, Command{Command: toyChat{}},
		ConfigRead{}, Hash{}, StableRead{}, Returned{},
	}
	for kind := KindInbound; kind <= KindReturned; kind++ { // the last kind of the register
		if _, ok := decoders[kind]; !ok {
			t.Errorf("the kind %d has no decoder", kind)
		}
	}
	for _, b := range bodies {
		if _, ok := decoders[b.Kind()]; !ok {
			t.Errorf("%T returns the kind %d, which has no decoder", b, b.Kind())
		}
	}
}

func TestTwoBodiesAreTheSameWhenTheyAreOfOneKindAndSayTheSame(t *testing.T) {
	move := toyMove{CharacterID: 1, Dir: 1, Ticks: 3}
	other := sim.StateID(12)
	same := [][2]Body{
		{Inbound{Message: benchMessage(7)}, Inbound{Message: benchMessage(7)}},
		{Command{Command: move}, Command{Command: move}},
		{ConfigRead{Version: 1}, ConfigRead{Version: 1}},
		{Hash{Value: 9}, Hash{Value: 9}},
		{StableRead{Source: other, CopyTick: 4}, StableRead{Source: other, CopyTick: 4}},
		{Returned{Reason: sim.Full}, Returned{Reason: sim.Full}},
	}
	for _, p := range same {
		if !p[0].Same(p[1]) {
			t.Errorf("%T: two bodies that say the same are not the same", p[0])
		}
	}
	different := [][2]Body{
		{Inbound{Message: benchMessage(7)}, Inbound{Message: benchMessage(8)}},
		{Command{Command: move}, Command{Command: toyMove{Ticks: 4}}},
		{ConfigRead{Version: 1}, ConfigRead{Version: 2}},
		{Hash{Value: 9}, Hash{Value: 10}},
		{StableRead{Source: other, CopyTick: 4}, StableRead{Source: other, CopyTick: 5}},
		{Returned{Reason: sim.Full}, Returned{Reason: sim.Retired}},
		{ConfigRead{Version: 9}, Hash{Value: 9}}, // the same number in another kind
	}
	for _, p := range different {
		if p[0].Same(p[1]) {
			t.Errorf("%T and %T: two bodies that differ are the same", p[0], p[1])
		}
	}
}
