package rand_test

import (
	"bytes"
	"testing"

	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/rand"
)

func draw(r *rand.Rand, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = r.Uint64()
	}
	return out
}

func equal(a, b []uint64) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return len(a) == len(b)
}

func TestTheSameSeedsGiveTheSameNumbersAndAnotherSeedDoesNot(t *testing.T) {
	if !equal(draw(rand.NewPCG(1, 2), 20), draw(rand.NewPCG(1, 2), 20)) {
		t.Fatal("two random numbers of the same seeds draw differently")
	}
	if equal(draw(rand.NewPCG(1, 2), 20), draw(rand.NewPCG(1, 3), 20)) {
		t.Fatal("two random numbers of other seeds draw the same")
	}
	if n := rand.NewPCG(1, 2).IntN(10); n < 0 || n >= 10 {
		t.Fatalf("IntN(10) = %d", n)
	}
}

func TestRandomNumbersThatWereWrittenGoOnFromWhereTheyWere(t *testing.T) {
	r := rand.NewPCG(7, 8)
	draw(r, 13) // somewhere in the middle
	var w binary.Writer
	if err := r.Encode(&w); err != nil {
		t.Fatal(err)
	}
	want := draw(r, 20)

	back, err := rand.Decode(binary.NewReader(w.B))
	if err != nil {
		t.Fatal(err)
	}
	if got := draw(back, 20); !equal(got, want) {
		t.Fatalf("the numbers that were read back are %v, want %v", got, want)
	}
	var again binary.Writer
	if err := back.Encode(&again); err != nil {
		t.Fatal(err)
	}
	var first binary.Writer
	r2 := rand.NewPCG(7, 8)
	draw(r2, 13+20)
	if err := r2.Encode(&first); err != nil || !bytes.Equal(first.B, again.B) {
		t.Fatalf("what was read back is written differently: %v", err)
	}
}

func TestDecodeRefusesWhatIsNotTheWrittenPlaceOfARandomNumber(t *testing.T) {
	var w binary.Writer
	rand.NewPCG(1, 2).Encode(&w)
	for n := range w.B { // every proper prefix
		if _, err := rand.Decode(binary.NewReader(w.B[:n])); err == nil {
			t.Fatalf("%d of %d bytes were read", n, len(w.B))
		}
	}
	var bad binary.Writer
	bad.String("not the place of a PCG")
	if _, err := rand.Decode(binary.NewReader(bad.B)); err == nil {
		t.Fatal("bytes that are not a place were read")
	}
}
