package config

import (
	"testing"
)

func TestAStoreThatTookNoDocumentHoldsTheDefaultsAtGenerationZero(t *testing.T) {
	cur := NewStore().Current()
	if cur.Generation != 0 || cur.Snapshot != Defaults() {
		t.Fatalf("current %+v", cur)
	}
	var none *Store
	if none.Current().Snapshot != Defaults() || none.Changed() != nil {
		t.Fatal("a nil store does not hold the defaults and signal nothing")
	}
}

func TestADocumentIsTakenWholeAndSignalsOnceWhateverTheNumberOfApplys(t *testing.T) {
	s := NewStore()
	doc := map[string]string{"config.test.unused": "1"}
	if err := s.Apply(doc); err != nil {
		t.Fatal(err)
	}
	if s.Current().Generation != 1 {
		t.Fatalf("generation %d, want 1", s.Current().Generation)
	}
	if err := s.Apply(doc); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Changed():
	default:
		t.Fatal("no signal after an Apply")
	}
	select {
	case <-s.Changed():
		t.Fatal("two Applys left two signals: a reader reads the latest anyway")
	default:
	}
}
