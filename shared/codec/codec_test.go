package codec_test

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/shared/codec"
)

type kind interface{ Number() uint16 }

type apple struct{}

func (apple) Number() uint16 { return 1 }

type pear struct{}

func (pear) Number() uint16 { return 2 }

type plum struct{}

func (plum) Number() uint16 { return 1 } // the number of apple

type reader func(b []byte) (kind, error)

func newRegistry() *codec.Registry[uint16, kind, reader] {
	return codec.New[uint16, kind, reader](kind.Number)
}

func TestAKindIsFoundByItsNumberAndTheNumbersAreInOrder(t *testing.T) {
	r := newRegistry()
	read := func([]byte) (kind, error) { return apple{}, nil }
	if err := r.Register(pear{}, read); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(apple{}, read); err != nil {
		t.Fatal(err)
	}
	if !r.Has(1) || !r.Has(2) || r.Has(3) {
		t.Fatal("Has does not follow what was registered")
	}
	if got := r.Numbers(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("numbers %v, want 1 and 2", got)
	}
	if f, ok := r.Lookup(2); !ok || f == nil {
		t.Fatal("Lookup did not find the kind with the number 2")
	}
}

func TestANumberOrAGoTypeThatTwoKindsShareIsRefused(t *testing.T) {
	r := newRegistry()
	read := func([]byte) (kind, error) { return apple{}, nil }
	if err := r.Register(apple{}, read); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(apple{}, read); !errors.Is(err, codec.ErrDuplicate) {
		t.Fatalf("the same type twice: %v", err)
	}
	if err := r.Register(plum{}, read); !errors.Is(err, codec.ErrDuplicate) {
		t.Fatalf("the number of another kind: %v", err)
	}
	if err := r.Register(pear{}, nil); !errors.Is(err, codec.ErrNoPrototype) {
		t.Fatalf("no function that reads: %v", err)
	}
	var none kind
	if err := r.Register(none, read); !errors.Is(err, codec.ErrNoPrototype) {
		t.Fatalf("no prototype: %v", err)
	}
	if r.Has(2) {
		t.Fatal("a kind that was refused is registered")
	}
}
