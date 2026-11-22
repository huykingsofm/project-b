package onfail

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

func TestEveryIDOfTheRegisterHasADecoder(t *testing.T) {
	for id := RetryID; id <= DropID; id++ { // the last ID of the register
		if _, ok := decoders[id]; !ok {
			t.Errorf("the kind with the ID %d has no decoder", id)
		}
	}
}

func TestAKindThatHoldsNoMessageComesBackAsItWasAndCutBytesAreAnError(t *testing.T) {
	for _, f := range []message.OnFail{
		Retry{}, Drop{},
		Redirect{To: message.ZoneLayerAddress(6)}, Escalate{To: message.ZoneAddress(1)},
		Notify{Key: "item.gone", Args: []string{"sword", "3"}}, Notify{Key: "item.gone"},
	} {
		var w binarycodec.Writer
		if err := Encode(&w, f); err != nil {
			t.Fatalf("%T: %v", f, err)
		}
		r := binarycodec.NewReader(w.B)
		got, err := Decode(r, 1, nil)
		if err != nil || r.Left() != 0 || !sameKind(got, f) {
			t.Fatalf("%T came back as %+v, error %v, %d bytes left", f, got, err, r.Left())
		}
		for n := range w.B { // every proper prefix
			if f.ID() == RetryID || f.ID() == DropID {
				break // nothing but the ID, which is all of the bytes
			}
			if _, err := Decode(binarycodec.NewReader(w.B[:n]), 1, nil); err == nil {
				t.Fatalf("%T: %d of %d bytes were read", f, n, len(w.B))
			}
		}
	}
}

func sameKind(a, b message.OnFail) bool {
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
	var w binarycodec.Writer
	if err := Encode(&w, nil); !errors.Is(err, message.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a nil OnFail: %v, %d bytes written", err, len(w.B))
	}
	if err := Encode(&w, Compensate{}); !errors.Is(err, message.ErrNoCodec) || len(w.B) != 0 {
		t.Fatalf("a compensation with no message: %v, %d bytes written", err, len(w.B))
	}
	w.Uvarint(99)
	_, err := Decode(binarycodec.NewReader(w.B), 1, nil)
	if !errors.Is(err, message.ErrUnknownCode) {
		t.Fatalf("the number 99: %v, want ErrUnknownCode", err)
	}
}
