package ids_test

import (
	"math/rand/v2"
	"testing"

	"github.com/huykingsofm/project-b/shared/codec/binary"
	"github.com/huykingsofm/project-b/shared/ids"
)

type testID uint32

func give(t *testing.T, a *ids.Allocator[testID]) testID {
	t.Helper()
	id, ok := a.Give()
	if !ok {
		t.Fatal("Give: no ID")
	}
	return id
}

func TestGiveCountsFromOneAndNeverGivesZero(t *testing.T) {
	a := ids.New[testID](3, 100)
	for want := testID(1); want <= 5; want++ {
		if got := give(t, &a); got != want || a.Status(got) != ids.Live {
			t.Fatalf("Give = %d (%v), want %d", got, a.Status(got), want)
		}
	}
	if a.Status(0) != ids.NotGiven || a.LiveCount() != 5 {
		t.Fatalf("Status(0) = %v, LiveCount = %d", a.Status(0), a.LiveCount())
	}
}

func TestIDsAreGivenInOrderUpToTheLimitBeforeOneIsGivenAgain(t *testing.T) {
	a := ids.New[testID](0, 5)
	one := give(t, &a)
	a.Release(one)
	for want := testID(2); want <= 5; want++ {
		if got := give(t, &a); got != want {
			t.Fatalf("Give = %d, want %d: an ID that was given back came first", got, want)
		}
	}
	if got := give(t, &a); got != one || !a.Wrapped() {
		t.Fatalf("Give after the limit = %d, want the one given back, %d", got, one)
	}
}

func TestAfterGoingRoundAnIDThatIsLiveOrInTheGraveIsSkipped(t *testing.T) {
	a := ids.New[testID](10, 4)
	for range 4 {
		give(t, &a)
	}
	a.Release(2)
	a.Release(3)
	if _, ok := a.Give(); ok {
		t.Fatal("an ID was given while every one is live or in the grave")
	}
	for range 10 {
		a.Step()
	}
	if got := give(t, &a); got != 2 {
		t.Fatalf("Give = %d, want 2: 1 is live", got)
	}
}

func TestAnIDWaitsInTheGraveForTheSteps(t *testing.T) {
	a := ids.New[testID](3, 2)
	one, two := give(t, &a), give(t, &a)
	a.Release(one)
	if a.Status(one) != ids.InGrave || a.Status(two) != ids.Live {
		t.Fatalf("Status = %v and %v", a.Status(one), a.Status(two))
	}
	for step := 1; step <= 2; step++ {
		a.Step()
		if _, ok := a.Give(); ok {
			t.Fatalf("an ID came back after %d steps of 3", step)
		}
	}
	a.Step()
	if a.Status(one) != ids.NotGiven {
		t.Fatalf("Status after 3 steps = %v", a.Status(one))
	}
	if got := give(t, &a); got != one {
		t.Fatalf("Give = %d, want %d", got, one)
	}
}

func TestReleaseOfAnIDThatIsNotLiveChangesNothing(t *testing.T) {
	a := ids.New[testID](2, 100)
	one := give(t, &a)
	for name, id := range map[string]testID{"never given": 9, "zero": 0, "above the limit": 101} {
		if a.Release(id) {
			t.Errorf("Release of an ID that is %s succeeded", name)
		}
	}
	a.Release(one)
	if a.Release(one) {
		t.Fatal("Release of an ID in the grave succeeded")
	}
	if a.GraveLen() != 1 || a.LiveCount() != 0 {
		t.Fatalf("GraveLen = %d, LiveCount = %d", a.GraveLen(), a.LiveCount())
	}
}

func TestStepsOfZeroKeepNothing(t *testing.T) {
	a := ids.New[testID](0, 100)
	one := give(t, &a)
	if !a.Release(one) || a.GraveLen() != 0 || a.Status(one) != ids.NotGiven {
		t.Fatalf("Status = %v, %d in the grave", a.Status(one), a.GraveLen())
	}
}

// run does a long run of calls chosen by a generator with a seed, and returns what each gave.
func run(seed uint64, a *ids.Allocator[testID]) []testID {
	rng := rand.New(rand.NewPCG(seed, 1))
	var held, out []testID
	for range 3000 {
		switch rng.IntN(4) {
		case 0, 1:
			if id, ok := a.Give(); ok {
				held = append(held, id)
				out = append(out, id)
			}
		case 2:
			if len(held) > 0 {
				i := rng.IntN(len(held))
				a.Release(held[i])
				held = append(held[:i], held[i+1:]...)
			}
		default:
			a.Step()
		}
	}
	return out
}

func TestTheSameCallsGiveTheSameIDs(t *testing.T) {
	a, b := ids.New[testID](5, 200), ids.New[testID](5, 200)
	x, y := run(7, &a), run(7, &b)
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
	a := ids.New[testID](steps, 50) // a small limit, to go round many times
	rng := rand.New(rand.NewPCG(3, 3))
	held := map[testID]bool{}
	releasedAt := map[testID]uint64{}
	var order []testID
	for range 20000 {
		switch rng.IntN(4) {
		case 0, 1:
			id, ok := a.Give()
			if !ok {
				continue
			}
			if held[id] {
				t.Fatalf("%d was given while live", id)
			}
			if at, was := releasedAt[id]; was && a.Clock()-at < steps {
				t.Fatalf("%d was given again %d steps after it was given back", id, a.Clock()-at)
			}
			held[id] = true
			order = append(order, id)
		case 2:
			if len(order) == 0 {
				continue
			}
			i := rng.IntN(len(order))
			a.Release(order[i])
			held[order[i]] = false
			releasedAt[order[i]] = a.Clock()
			order = append(order[:i], order[i+1:]...)
		default:
			a.Step()
		}
	}
	if !a.Wrapped() {
		t.Fatal("the run never went round, so it tested nothing of skipping")
	}
	if a.LiveCount() != len(order) {
		t.Fatalf("LiveCount = %d, %d are held", a.LiveCount(), len(order))
	}
}

func TestCloneSharesNothing(t *testing.T) {
	a := ids.New[testID](2, 100)
	one := give(t, &a)
	c := a.Clone()
	a.Release(one)
	if c.Status(one) != ids.Live || a.Status(one) != ids.InGrave {
		t.Fatalf("Status = %v in the clone, %v in the original", c.Status(one), a.Status(one))
	}
}

func encoded(a *ids.Allocator[testID]) []byte {
	var w binary.Writer
	a.Encode(&w)
	return w.B
}

func TestAnAllocatorSurvivesEncodeAndDecodeWhenItsOwnerMarksWhatIsLive(t *testing.T) {
	a := ids.New[testID](4, 50)
	run(11, &a)
	got, err := ids.Decode[testID](binary.NewReader(encoded(&a)))
	if err != nil {
		t.Fatal(err)
	}
	if got.LiveCount() != 0 {
		t.Fatalf("%d IDs are live before the owner marks them", got.LiveCount())
	}
	for id := testID(1); id <= a.Limit(); id++ {
		if a.IsLive(id) {
			if err := got.MarkLive(id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if string(encoded(&got)) != string(encoded(&a)) || got.LiveCount() != a.LiveCount() {
		t.Fatal("what was read is not what was written")
	}
	for range 50 {
		x, okx := a.Give()
		y, oky := got.Give()
		if x != y || okx != oky {
			t.Fatalf("Give = %d, %v and %d, %v", x, okx, y, oky)
		}
		a.Step()
		got.Step()
	}
}

func TestMarkLiveRefusesAnIDThatCannotBeLive(t *testing.T) {
	a := ids.New[testID](2, 50)
	one, two := give(t, &a), give(t, &a)
	a.Release(one)
	got, err := ids.Decode[testID](binary.NewReader(encoded(&a)))
	if err != nil {
		t.Fatal(err)
	}
	if err := got.MarkLive(two); err != nil {
		t.Fatalf("a live ID was refused: %v", err)
	}
	for name, id := range map[string]testID{
		"zero": 0, "never given": 9, "in the grave": one, "marked already": two,
		"above the limit": 51,
	} {
		if err := got.MarkLive(id); err == nil {
			t.Errorf("an ID %s was marked live", name)
		}
	}
}

func TestDecodeRefusesWhatIsNotAnAllocator(t *testing.T) {
	a := ids.New[testID](2, 50)
	one, two := give(t, &a), give(t, &a)
	give(t, &a)
	a.Release(one)
	a.Release(two)
	good := encoded(&a)
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
		"an ID never given in the grave": func(w *binary.Writer) {
			w.Uvarint(1)
			w.Uvarint(9)
			w.Uvarint(2)
		},
		"a step that is not after the clock": func(w *binary.Writer) {
			w.Uvarint(1)
			w.Uvarint(1)
			w.Uvarint(0)
		},
		"a count too large": func(w *binary.Writer) { w.Uvarint(1<<40 | 1) },
	}
	for name, rest := range cases {
		var w binary.Writer
		header(&w)
		rest(&w)
		if _, err := ids.Decode[testID](binary.NewReader(w.B)); err == nil {
			t.Errorf("%s was read", name)
		}
	}
}
