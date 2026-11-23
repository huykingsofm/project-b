package ids_test

import (
	"math/rand/v2"
	"testing"

	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ids"
)

type testID uint32

// owner is what uses the IDs, as a state does: it knows which ones it holds.
type owner struct {
	a    ids.Allocator[testID]
	used map[testID]bool
}

func newOwner(steps uint64, limit testID) *owner {
	return &owner{a: ids.New(steps, limit), used: map[testID]bool{}}
}

func (o *owner) inUse(id testID) bool { return o.used[id] }

func (o *owner) give() (testID, bool) {
	id, ok := o.a.Give(o.inUse, len(o.used))
	if ok {
		o.used[id] = true
	}
	return id, ok
}

func (o *owner) mustGive(t *testing.T) testID {
	t.Helper()
	id, ok := o.give()
	if !ok {
		t.Fatal("Give: no ID")
	}
	return id
}

func (o *owner) release(id testID) bool {
	if !o.used[id] {
		return false
	}
	delete(o.used, id)
	return o.a.Release(id)
}

func TestGiveCountsFromOneAndNeverGivesZero(t *testing.T) {
	o := newOwner(3, 100)
	for want := testID(1); want <= 5; want++ {
		if got := o.mustGive(t); got != want {
			t.Fatalf("Give = %d, want %d", got, want)
		}
	}
	if o.a.Status(0, o.inUse) != ids.NotGiven {
		t.Fatalf("Status(0) = %v", o.a.Status(0, o.inUse))
	}
}

func TestIDsAreGivenInOrderUpToTheLimitBeforeOneIsGivenAgain(t *testing.T) {
	o := newOwner(0, 5)
	one := o.mustGive(t)
	o.release(one)
	for want := testID(2); want <= 5; want++ {
		if got := o.mustGive(t); got != want {
			t.Fatalf("Give = %d, want %d: an ID that was given back came first", got, want)
		}
	}
	if got := o.mustGive(t); got != one {
		t.Fatalf("Give after the limit = %d, want the one that was given back, %d", got, one)
	}
	if !o.a.Wrapped() {
		t.Fatal("the allocator does not say that it went round")
	}
}

func TestAfterGoingRoundAnIDInUseOrInTheGraveIsSkipped(t *testing.T) {
	o := newOwner(10, 4)
	for range 4 {
		o.mustGive(t)
	}
	o.release(2) // in the grave
	o.release(3) // in the grave too
	o.a = o.a.Clone()
	if _, ok := o.give(); ok {
		t.Fatal("an ID was given while every one is in use or in the grave")
	}
	for range 10 {
		o.a.Step()
	}
	if got := o.mustGive(t); got != 2 {
		t.Fatalf("Give = %d, want 2: 1 is in use", got)
	}
}

func TestAnIDWaitsInTheGraveForTheSteps(t *testing.T) {
	o := newOwner(3, 2)
	one, two := o.mustGive(t), o.mustGive(t)
	o.release(one)
	if o.a.Status(one, o.inUse) != ids.InGrave || o.a.Status(two, o.inUse) != ids.Live {
		t.Fatalf("Status = %v and %v", o.a.Status(one, o.inUse), o.a.Status(two, o.inUse))
	}
	for step := 1; step <= 2; step++ {
		o.a.Step()
		if _, ok := o.give(); ok {
			t.Fatalf("an ID came back after %d steps of 3", step)
		}
	}
	o.a.Step()
	if o.a.Status(one, o.inUse) != ids.NotGiven {
		t.Fatalf("Status after 3 steps = %v", o.a.Status(one, o.inUse))
	}
	if got := o.mustGive(t); got != one {
		t.Fatalf("Give = %d, want %d", got, one)
	}
}

func TestReleaseOfAnIDThatCannotHaveBeenGivenChangesNothing(t *testing.T) {
	o := newOwner(2, 100)
	one := o.mustGive(t)
	for name, id := range map[string]testID{"never given": 9, "zero": 0, "above the limit": 101} {
		if o.a.Release(id) {
			t.Errorf("Release of an ID that is %s succeeded", name)
		}
	}
	o.release(one)
	if o.a.Release(one) {
		t.Fatal("Release of an ID in the grave succeeded")
	}
	if o.a.GraveLen() != 1 {
		t.Fatalf("GraveLen = %d", o.a.GraveLen())
	}
}

func TestStepsOfZeroKeepNothing(t *testing.T) {
	o := newOwner(0, 100)
	one := o.mustGive(t)
	if !o.release(one) || o.a.GraveLen() != 0 || o.a.Status(one, o.inUse) != ids.NotGiven {
		t.Fatalf("Status = %v, %d in the grave", o.a.Status(one, o.inUse), o.a.GraveLen())
	}
}

// run does a long run of calls chosen by a generator with a seed, and returns what each gave.
func run(seed uint64, o *owner) []testID {
	rng := rand.New(rand.NewPCG(seed, 1))
	var held, out []testID
	for range 3000 {
		switch rng.IntN(4) {
		case 0, 1:
			if id, ok := o.give(); ok {
				held = append(held, id)
				out = append(out, id)
			}
		case 2:
			if len(held) > 0 {
				i := rng.IntN(len(held))
				o.release(held[i])
				held = append(held[:i], held[i+1:]...)
			}
		default:
			o.a.Step()
		}
	}
	return out
}

func TestTheSameCallsGiveTheSameIDs(t *testing.T) {
	x, y := run(7, newOwner(5, 200)), run(7, newOwner(5, 200))
	if len(x) == 0 || len(x) != len(y) {
		t.Fatalf("%d and %d IDs", len(x), len(y))
	}
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("ID %d: %d and %d", i, x[i], y[i])
		}
	}
}

func TestAnIDIsNeverLiveTwiceAndNeverGivenBeforeItsSteps(t *testing.T) {
	const steps = 5
	o := newOwner(steps, 50) // a small limit, to go round many times
	rng := rand.New(rand.NewPCG(3, 3))
	releasedAt := map[testID]uint64{}
	var held []testID
	for range 20000 {
		switch rng.IntN(4) {
		case 0, 1:
			id, ok := o.a.Give(o.inUse, len(o.used))
			if !ok {
				continue
			}
			if o.used[id] {
				t.Fatalf("%d was given while in use", id)
			}
			if at, was := releasedAt[id]; was && o.a.Clock()-at < steps {
				t.Fatalf("%d was given again %d steps after it was given back", id, o.a.Clock()-at)
			}
			o.used[id] = true
			held = append(held, id)
		case 2:
			if len(held) == 0 {
				continue
			}
			i := rng.IntN(len(held))
			o.release(held[i])
			releasedAt[held[i]] = o.a.Clock()
			held = append(held[:i], held[i+1:]...)
		default:
			o.a.Step()
		}
	}
	if !o.a.Wrapped() {
		t.Fatal("the run never went round, so it tested nothing of skipping")
	}
}

func TestCloneSharesNothing(t *testing.T) {
	o := newOwner(2, 100)
	one := o.mustGive(t)
	o.release(one)
	c := o.a.Clone()
	o.a.Step()
	o.a.Step()
	if !c.InGrave(one) || o.a.InGrave(one) {
		t.Fatalf(
			"in the grave: %v in the clone, %v in the original", c.InGrave(one), o.a.InGrave(one),
		)
	}
}

func encoded(a *ids.Allocator[testID]) []byte {
	var w binary.Writer
	a.Encode(&w)
	return w.B
}

func TestAnAllocatorSurvivesEncodeAndDecode(t *testing.T) {
	o := newOwner(4, 50)
	run(11, o)
	got, err := ids.Decode[testID](binary.NewReader(encoded(&o.a)))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded(&got)) != string(encoded(&o.a)) {
		t.Fatal("what was read does not encode to what was written")
	}
	other := &owner{a: got, used: map[testID]bool{}}
	for id := range o.used {
		other.used[id] = true
	}
	for range 50 {
		x, okx := o.give()
		y, oky := other.give()
		if x != y || okx != oky {
			t.Fatalf("Give = %d, %v and %d, %v", x, okx, y, oky)
		}
		o.a.Step()
		other.a.Step()
	}
}

func TestDecodeRefusesWhatIsNotAnAllocator(t *testing.T) {
	o := newOwner(2, 50)
	one, two := o.mustGive(t), o.mustGive(t)
	o.release(one)
	o.release(two)
	good := encoded(&o.a)
	for n := range good {
		if _, err := ids.Decode[testID](binary.NewReader(good[:n])); err == nil {
			t.Fatalf("a cut at %d bytes of %d was read", n, len(good))
		}
	}
	header := func(w *binary.Writer) {
		w.Uvarint(2)  // steps
		w.Uvarint(50) // limit
		w.Uvarint(2)  // last
		w.Bool(false) // wrapped
		w.Uvarint(0)  // clock
	}
	cases := map[string]func(w *binary.Writer){
		"an ID twice in the grave": func(w *binary.Writer) {
			w.Uvarint(2)
			w.Uvarint(1)
			w.Uvarint(2)
			w.Uvarint(1)
			w.Uvarint(2)
		},
		"an ID never given": func(w *binary.Writer) {
			w.Uvarint(1)
			w.Uvarint(9)
			w.Uvarint(2)
		},
		"a count too large": func(w *binary.Writer) { w.Uvarint(1<<40 | 1) },
	}
	for name, grave := range cases {
		var w binary.Writer
		header(&w)
		grave(&w)
		if _, err := ids.Decode[testID](binary.NewReader(w.B)); err == nil {
			t.Errorf("%s was read", name)
		}
	}
}
