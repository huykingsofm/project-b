package eventlog

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestEveryKindOfTheRegisterHasADecoderAndABodyThatReturnsIt(t *testing.T) {
	bodies := []Body{
		Inbound{Message: benchMessage(1)}, Command{Command: toyChat{}},
		ConfigRead{}, Hash{}, StableRead{}, Returned{}, Retired{},
	}
	for kind := KindInbound; kind <= KindRetired; kind++ { // the last kind of the register
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
		{Returned{Reason: sim.DeliveryFailureFull}, Returned{Reason: sim.DeliveryFailureFull}},
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
		{Returned{Reason: sim.DeliveryFailureFull}, Returned{Reason: sim.DeliveryFailureRetired}},
		{ConfigRead{Version: 9}, Hash{Value: 9}}, // the same number in another kind
	}
	for _, p := range different {
		if p[0].Same(p[1]) {
			t.Errorf("%T and %T: two bodies that differ are the same", p[0], p[1])
		}
	}
}

func TestOnlyAMessageAndAReadOfACopyMakeAnEngineDependOnAnother(t *testing.T) {
	stamped := benchMessage(1).Stamp(sim.Header{From: 4, Seq: 9, Tick: 30})
	dep := func(source sim.StateID, tick uint64) sim.StateDependency {
		return sim.StateDependency{Source: source, Tick: tick}
	}
	none := sim.StateDependency{}
	for name, tc := range map[string]struct {
		body Body
		want sim.StateDependency
		ok   bool
	}{
		"a message":             {Inbound{Message: stamped}, dep(4, 30), true},
		"a message not stamped": {Inbound{Message: benchMessage(1)}, none, false},
		"a read of a copy":      {StableRead{Source: 5, CopyTick: 12}, dep(5, 12), true},
		"a read with no copy":   {StableRead{Source: 5}, none, false},
		"a hash":                {Hash{Value: 1}, none, false},
		"a command":             {Command{Command: toyChat{}}, none, false},
		"a configuration":       {ConfigRead{Version: 2}, none, false},
		"a message back":        {Returned{Header: sim.Header{From: 1, Tick: 5}}, none, false},
	} {
		got, ok := Record{Body: tc.body}.Dependency()
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%s: Dependency = %v, %v; want %v, %v", name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTheTickOfTheSenderIsPartOfTheHeaderAndComesBack(t *testing.T) {
	var w binary.Writer
	want := sim.Header{From: 7, Seq: 300, Tick: 1 << 40}
	want.Encode(&w)
	r := binary.NewReader(w.B)
	if got := sim.DecodeHeader(r); got != want || r.Left() != 0 {
		t.Fatalf("the header came back as %+v with %d bytes left", got, r.Left())
	}
}
