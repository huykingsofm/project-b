package onfail_test

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func registered(t *testing.T) *sim.Codecs {
	t.Helper()
	c := sim.NewCodecs()
	if err := onfail.Register(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryIDOfTheRegisterHasADecoder(t *testing.T) {
	c := registered(t)
	for id := onfail.RetryID; id <= onfail.DropID; id++ { // the last ID of the register
		if !c.OnFails.Has(id) {
			t.Errorf("the kind with the ID %d has no decoder", id)
		}
	}
}

func TestAKindThatHoldsNoMessageComesBackAsItWasAndCutBytesAreAnError(t *testing.T) {
	c := registered(t)
	for _, f := range []sim.OnFail{
		onfail.Retry{}, onfail.Drop{},
		onfail.Redirect{To: 6}, onfail.Escalate{To: 1},
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
			if f.ID() == onfail.RetryID || f.ID() == onfail.DropID {
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
	case onfail.Redirect:
		b, ok := b.(onfail.Redirect)
		return ok && a == b
	case onfail.Escalate:
		b, ok := b.(onfail.Escalate)
		return ok && a == b
	}
	return a.ID() == b.ID()
}

func TestANilOnFailAndANumberThatIsNoKindAreRefused(t *testing.T) {
	c := registered(t)
	var w binary.Writer
	if err := sim.EncodeOnFail(&w, nil); !errors.Is(err, sim.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a nil OnFail: %v, %d bytes written", err, len(w.B))
	}
	if err := sim.EncodeOnFail(&w, onfail.Compensate{}); !errors.Is(err,
		sim.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a compensation with no message: %v, %d bytes written", err, len(w.B))
	}
	w.Uvarint(99)
	_, err := c.DecodeOnFail(binary.NewReader(w.B), 1, nil)
	if !errors.Is(err, sim.ErrUnknownCode) {
		t.Fatalf("the number 99: %v, want ErrUnknownCode", err)
	}
}
