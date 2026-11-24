package engine

import (
	"errors"
	"testing"

	"github.com/huykingsofm/project-b/platform/sim"
)

func TestTheHashOfAPeerThatCannotBeWrittenSaysSoAndStillTellsItFromAnother(t *testing.T) {
	var good peerData
	bad := good
	bad.unsettledMessages.AddUnsent(nil) // a message that has no codec
	a, b := sim.NewHasher(), sim.NewHasher()
	if err := good.Hash(&a); err != nil {
		t.Fatal(err)
	}
	if err := bad.Hash(&b); !errors.Is(err, sim.ErrNoCodec) {
		t.Fatalf("the hash of a peer that cannot be written: %v, want ErrNoCodec", err)
	}
	if a == b {
		t.Fatal("a peer that cannot be written has the hash of one that can")
	}
}
