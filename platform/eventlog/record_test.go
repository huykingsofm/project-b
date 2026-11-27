package eventlog_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func TestTwoBodiesAreTheSameWhenTheyAreOfOneKindAndSayTheSame(t *testing.T) {
	move := toyMove{CharacterID: 1, Dir: 1, Ticks: 3}
	other := sim.StateID(12)
	same := [][2]eventlog.Body{
		{
			eventlog.Retired{By: sim.StateDependency{Source: 2, Tick: 5}},
			eventlog.Retired{By: sim.StateDependency{Source: 2, Tick: 5}},
		},
		{eventlog.Inbound{Message: benchMessage(7)}, eventlog.Inbound{Message: benchMessage(7)}},
		{eventlog.Command{Command: move}, eventlog.Command{Command: move}},
		{eventlog.Hash{Value: 9}, eventlog.Hash{Value: 9}},
		{eventlog.StableRead{Source: other, CopyTick: 4}, eventlog.StableRead{Source: other,
			CopyTick: 4}},
		{eventlog.Returned{Reason: sim.DeliveryFailureFull},
			eventlog.Returned{Reason: sim.DeliveryFailureFull}},
		{
			eventlog.Started{Creator: 2, CreatorTick: 5, Snapshot: []byte{9}},
			eventlog.Started{Creator: 2, CreatorTick: 5, Snapshot: []byte{9}},
		},
	}
	for _, p := range same {
		if !p[0].Same(p[1]) {
			t.Errorf("%T: two bodies that say the same are not the same", p[0])
		}
	}
	different := [][2]eventlog.Body{
		{
			eventlog.Retired{By: sim.StateDependency{Source: 2, Tick: 5}},
			eventlog.Retired{By: sim.StateDependency{Source: 2, Tick: 6}},
		},
		{eventlog.Inbound{Message: benchMessage(7)}, eventlog.Inbound{Message: benchMessage(8)}},
		{eventlog.Command{Command: move}, eventlog.Command{Command: toyMove{Ticks: 4}}},
		{eventlog.Hash{Value: 9}, eventlog.Hash{Value: 10}},
		{eventlog.StableRead{Source: other, CopyTick: 4}, eventlog.StableRead{Source: other,
			CopyTick: 5}},
		{eventlog.Returned{Reason: sim.DeliveryFailureFull},
			eventlog.Returned{Reason: sim.DeliveryFailureRetired}},
		{
			eventlog.Started{Creator: 2, CreatorTick: 5, Snapshot: []byte{9}},
			eventlog.Started{Creator: 2, CreatorTick: 6, Snapshot: []byte{9}},
		},
		{
			eventlog.Started{Creator: 2, CreatorTick: 5, Snapshot: []byte{9}},
			eventlog.Started{Creator: 2, CreatorTick: 5, Snapshot: []byte{10}},
		},
		{eventlog.Hash{Value: 9}, eventlog.StableRead{Source: 9,
			CopyTick: 9}}, // the same number in another kind
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
		body eventlog.Body
		want sim.StateDependency
		ok   bool
	}{
		"a message":             {eventlog.Inbound{Message: stamped}, dep(4, 30), true},
		"a message not stamped": {eventlog.Inbound{Message: benchMessage(1)}, none, false},
		"a read of a copy":      {eventlog.StableRead{Source: 5, CopyTick: 12}, dep(5, 12), true},
		"a read with no copy":   {eventlog.StableRead{Source: 5}, none, false},
		"a hash":                {eventlog.Hash{Value: 1}, none, false},
		"a command":             {eventlog.Command{Command: toyChat{}}, none, false},
		"a message back": {eventlog.Returned{Header: sim.Header{From: 1, Tick: 5}}, none,
			false},
	} {
		got, ok := eventlog.Record{Body: tc.body}.Dependency()
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
