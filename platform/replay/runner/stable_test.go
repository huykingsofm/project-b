package runner

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// stampState is a state that says which tick it was published at.
type stampState struct {
	id   sim.StateID
	tick uint64
}

func (s *stampState) ID() sim.StateID           { return s.id }
func (*stampState) Type() sim.StateType         { return 1 }
func (*stampState) String() string              { return "stamp" }
func (s *stampState) Tick() uint64              { return s.tick }
func (s *stampState) NextTick()                 { s.tick++ }
func (s *stampState) Clone() sim.State          { return s }
func (*stampState) Hash() uint64                { return 0 }
func (*stampState) TakeEvents() []sim.Event     { return nil }
func (*stampState) Encode(*binary.Writer) error { return nil }

// The copies that a replay keeps are those that a read still to come will ask for: when the run is
// over every read has been made, and none is kept.
func TestACopyIsLetGoOfAsSoonAsItsLastReadIsMade(t *testing.T) {
	// engine 2 published at ticks 3 and 4; engine 1 reads the copy of tick 3 twice (at ticks 5 and
	// 6) and the copy of tick 4 once (at tick 7)
	uses := map[copyKey]int{{2, 3}: 2, {2, 4}: 1}
	s := newReplayStable(uses)
	for tick := uint64(1); tick <= 5; tick++ {
		s.Publish(&stampState{id: 2, tick: tick}) // only the ones that a read asks for are kept
	}
	if s.held() != 2 {
		t.Fatalf("%d copies kept, want the 2 that a read will ask for", s.held())
	}
	s.expect(readKey{1, 5, 2}, 3)
	s.expect(readKey{1, 6, 2}, 3)
	s.expect(readKey{1, 7, 2}, 4)

	view := s.View(1, replay.NopLog{})
	for _, c := range []struct {
		tick uint64
		want uint64
		held int
	}{{5, 3, 2}, {6, 3, 1}, {7, 4, 0}} {
		got := s.Copy(1, c.tick, 2)
		if got == nil || got.Tick() != c.want {
			t.Fatalf("the read at %d found %v, want the copy of tick %d", c.tick, got, c.want)
		}
		if s.held() != c.held {
			t.Fatalf("after the read at %d, %d copies are kept, want %d", c.tick, s.held(), c.held)
		}
	}
	if view == nil || len(s.uses) != 0 || len(s.reads) != 0 {
		t.Fatalf(
			"%d copies still wait for a read and %d reads are to come", len(s.uses), len(s.reads),
		)
	}
	if got := s.Copy(1, 9, 2); got != nil {
		t.Fatal("a read that the log does not have found a copy")
	}
}
