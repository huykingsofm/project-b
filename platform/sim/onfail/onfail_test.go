package onfail

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func registered(t *testing.T) *sim.Codecs {
	t.Helper()
	c := sim.NewCodecs()
	if err := Register(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryIDOfTheRegisterHasADecoder(t *testing.T) {
	c := registered(t)
	for id := RetryID; id <= DropID; id++ { // the last ID of the register
		if !c.OnFails.Has(id) {
			t.Errorf("the kind with the ID %d has no decoder", id)
		}
	}
}

func TestAKindThatHoldsNoMessageComesBackAsItWasAndCutBytesAreAnError(t *testing.T) {
	c := registered(t)
	for _, f := range []sim.OnFail{
		Retry{}, Drop{},
		Redirect{To: 6}, Escalate{To: 1},
		Notify{Key: "item.gone", Args: []string{"sword", "3"}}, Notify{Key: "item.gone"},
	} {
		var w binary.Writer
		if err := sim.EncodeOnFail(&w, f); err != nil {
			t.Fatalf("%T: %v", f, err)
		}
		r := binary.NewReader(w.B)
		got, err := c.DecodeOnFail(r, 1, nil)
		if err != nil || r.Left() != 0 || !sameKind(got, f) {
			t.Fatalf("%T came back as %+v, error %v, %d bytes left", f, got, err, r.Left())
		}
		for n := range w.B { // every proper prefix
			if f.ID() == RetryID || f.ID() == DropID {
				break // nothing but the ID, which is all of the bytes
			}
			if _, err := c.DecodeOnFail(binary.NewReader(w.B[:n]), 1, nil); err == nil {
				t.Fatalf("%T: %d of %d bytes were read", f, n, len(w.B))
			}
		}
	}
}

func sameKind(a, b sim.OnFail) bool {
	switch a := a.(type) {
	case Redirect:
		b, ok := b.(Redirect)
		return ok && a == b
	case Escalate:
		b, ok := b.(Escalate)
		return ok && a == b
	case Notify:
		b, ok := b.(Notify)
		return ok && a.Key == b.Key && len(a.Args) == len(b.Args)
	}
	return a.ID() == b.ID()
}

func TestANilOnFailAndANumberThatIsNoKindAreRefused(t *testing.T) {
	c := registered(t)
	var w binary.Writer
	if err := sim.EncodeOnFail(&w, nil); !errors.Is(err, sim.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a nil OnFail: %v, %d bytes written", err, len(w.B))
	}
	if err := sim.EncodeOnFail(&w, Compensate{}); !errors.Is(err, sim.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a compensation with no message: %v, %d bytes written", err, len(w.B))
	}
	w.Uvarint(99)
	_, err := c.DecodeOnFail(binary.NewReader(w.B), 1, nil)
	if !errors.Is(err, sim.ErrUnknownCode) {
		t.Fatalf("the number 99: %v, want ErrUnknownCode", err)
	}
}

// aBouncer counts what an OnFail asks of the engine that sent the message.
type aBouncer struct{ resent, dropped, told int }

func (b *aBouncer) Resend(sim.Message)         { b.resent++ }
func (b *aBouncer) Tell(any, string, []string) { b.told++ }
func (b *aBouncer) Drop()                      { b.dropped++ }
func (*aBouncer) Stopping() bool               { return false }
func (*aBouncer) MaxAttempts() int             { return 3 }

// aMessage is a message that holds nothing, for an OnFail to be run on.
type aMessage struct{ attempts int }

func (aMessage) Dest() sim.StateID                    { return 1 }
func (aMessage) Head() sim.Header                     { return sim.Header{} }
func (aMessage) Subject() any                         { return nil }
func (aMessage) OnFail() sim.OnFail                   { return nil }
func (m aMessage) Attempts() int                      { return m.attempts }
func (m aMessage) Stamp(sim.Header) sim.Message       { return m }
func (m aMessage) Again() sim.Message                 { m.attempts++; return m }
func (m aMessage) Redirected(sim.StateID) sim.Message { return m }
func (aMessage) Encode(*binary.Writer) error          { return nil }

func TestRetrySendsAgainForALostChanceButNeverToAStateThatIsGone(t *testing.T) {
	m := aMessage{}
	for _, tc := range []struct {
		reason  sim.Reason
		resends int
	}{
		{sim.Full, 1}, {sim.NotStarted, 1}, {sim.Moved, 1}, {sim.Retired, 0}, {sim.NotGiven, 0},
	} {
		var b aBouncer
		Retry{}.Handle(&b, m, tc.reason)
		if b.resent != tc.resends || b.dropped != 1-tc.resends {
			t.Errorf("for %v: %d resent and %d dropped, want %d resent", tc.reason,
				b.resent, b.dropped, tc.resends)
		}
	}
}

func TestRedirectAndEscalateStillGoToAnotherEngineWhenTheStateIsGone(t *testing.T) {
	m := aMessage{}
	for _, f := range []sim.OnFail{Redirect{To: 6}, Escalate{To: 2}} {
		var b aBouncer
		f.Handle(&b, m, sim.Retired)
		if b.resent != 1 {
			t.Errorf("%T did not give the message to the other engine", f)
		}
	}
}
