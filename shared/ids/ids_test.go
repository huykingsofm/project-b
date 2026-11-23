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
		if got := give(t, &a); got != want {
			t.Fatalf("Give = %d, want %d", got, want)
		}
	}
	if a.Status(0) != ids.NeverGiven {
		t.Fatalf("Status(0) = %v", a.Status(0))
	}
}

func TestAnIDWaitsInTheGraveForTheStepsAndIsThenGivenAgain(t *testing.T) {
	a := ids.New[testID](3, 100)
	one, two := give(t, &a), give(t, &a)
	if !a.Release(one) {
		t.Fatal("Release of a live ID failed")
	}
	if a.Status(one) != ids.InGrave || a.Status(two) != ids.Live {
		t.Fatalf("Status = %v and %v", a.Status(one), a.Status(two))
	}
	for step := 1; step <= 2; step++ {
		a.Step()
		if got := give(t, &a); got == one {
			t.Fatalf("the ID came back after %d steps of 3", step)
		}
	}
	a.Step()
	if a.Status(one) != ids.Free {
		t.Fatalf("Status after 3 steps = %v", a.Status(one))
	}
	if got := give(t, &a); got != one {
		t.Fatalf("Give = %d, want the one that was given back, %d", got, one)
	}
	if a.Status(one) != ids.Live {
		t.Fatalf("Status after it was given again = %v", a.Status(one))
	}
}

func TestTheIDFreeLongestIsGivenFirst(t *testing.T) {
	a := ids.New[testID](0, 100)
	first, second, third := give(t, &a), give(t, &a), give(t, &a)
	a.Release(second)
	a.Release(third)
	a.Release(first)
	for _, want := range []testID{second, third, first} {
		if got := give(t, &a); got != want {
			t.Fatalf("Give = %d, want %d", got, want)
		}
	}
}

func TestStepsOfZeroMakeAnIDFreeAtOnce(t *testing.T) {
	a := ids.New[testID](0, 100)
	one := give(t, &a)
	a.Release(one)
	if a.Status(one) != ids.Free {
		t.Fatalf("Status = %v", a.Status(one))
	}
}

func TestReleaseOfAnIDThatIsNotLiveChangesNothing(t *testing.T) {
	a := ids.New[testID](2, 100)
	one := give(t, &a)
	for name, id := range map[string]testID{"never given": 9, "zero": 0} {
		if a.Release(id) {
			t.Fatalf("Release of an ID that is %s succeeded", name)
		}
	}
	a.Release(one)
	if a.Release(one) {
		t.Fatal("Release of an ID in the grave succeeded")
	}
	a.Step()
	a.Step()
	if a.Release(one) {
		t.Fatal("Release of a free ID succeeded")
	}
	if a.LiveCount() != 0 {
		t.Fatalf("LiveCount = %d", a.LiveCount())
	}
}

func TestNothingIsGivenAtTheLimitUntilAnIDIsFree(t *testing.T) {
	a := ids.New[testID](1, 2)
	one, _ := give(t, &a), give(t, &a)
	if _, ok := a.Give(); ok {
		t.Fatal("Give went past the limit")
	}
	a.Release(one)
	if _, ok := a.Give(); ok {
		t.Fatal("Give gave an ID that is in the grave")
	}
	a.Step()
	if got := give(t, &a); got != one {
		t.Fatalf("Give = %d, want %d", got, one)
	}
}

func TestTheLiveCountFollowsWhatIsGivenAndGivenBack(t *testing.T) {
	a := ids.New[testID](2, 100)
	one, two, three := give(t, &a), give(t, &a), give(t, &a)
	a.Release(one)
	a.Release(two)
	if a.LiveCount() != 1 {
		t.Fatalf("LiveCount = %d, want 1 (%d)", a.LiveCount(), three)
	}
}

// run does a long run of calls chosen by a generator with a seed, and returns what each gave.
func run(seed uint64, a *ids.Allocator[testID]) []testID {
	rng := rand.New(rand.NewPCG(seed, 1))
	var live, out []testID
	for range 2000 {
		switch rng.IntN(4) {
		case 0, 1:
			if id, ok := a.Give(); ok {
				live = append(live, id)
				out = append(out, id)
			}
		case 2:
			if len(live) > 0 {
				i := rng.IntN(len(live))
				a.Release(live[i])
				live = append(live[:i], live[i+1:]...)
			}
		default:
			a.Step()
		}
	}
	return out
}

func TestTheSameCallsGiveTheSameIDs(t *testing.T) {
	a, b := ids.New[testID](5, 1000), ids.New[testID](5, 1000)
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
	a := ids.New[testID](steps, 1000)
	rng := rand.New(rand.NewPCG(3, 3))
	live := map[testID]bool{}
	releasedAt := map[testID]uint64{}
	var given []testID
	for range 5000 {
		switch rng.IntN(4) {
		case 0, 1:
			id, ok := a.Give()
			if !ok {
				continue
			}
			if live[id] {
				t.Fatalf("%d was given while live", id)
			}
			if at, was := releasedAt[id]; was && a.Clock()-at < steps {
				t.Fatalf("%d was given again %d steps after it was given back", id, a.Clock()-at)
			}
			live[id] = true
			given = append(given, id)
		case 2:
			if len(given) == 0 {
				continue
			}
			if id := given[rng.IntN(len(given))]; live[id] {
				a.Release(id)
				live[id] = false
				releasedAt[id] = a.Clock()
			}
		default:
			a.Step()
		}
	}
}

func TestCloneSharesNothing(t *testing.T) {
	a := ids.New[testID](2, 100)
	one := give(t, &a)
	a.Release(one)
	c := a.Clone()
	a.Step()
	a.Step()
	if c.Status(one) != ids.InGrave || a.Status(one) != ids.Free {
		t.Fatalf("Status = %v in the clone, %v in the original", c.Status(one), a.Status(one))
	}
	give(t, &c)
	if a.Status(one) != ids.Free {
		t.Fatal("giving an ID in the clone changed the original")
	}
}

func encoded(a *ids.Allocator[testID]) []byte {
	var w binary.Writer
	a.Encode(&w)
	return w.B
}

func TestAnAllocatorSurvivesEncodeAndDecode(t *testing.T) {
	a := ids.New[testID](4, 50)
	run(11, &a)
	got, err := ids.Decode[testID](binary.NewReader(encoded(&a)))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded(&got)) != string(encoded(&a)) {
		t.Fatal("what was read does not encode to what was written")
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

func TestDecodeRefusesWhatIsNotAnAllocator(t *testing.T) {
	a := ids.New[testID](2, 50)
	one, two := give(t, &a), give(t, &a)
	a.Release(one)
	a.Release(two)
	good := encoded(&a)
	for n := range good {
		if _, err := ids.Decode[testID](binary.NewReader(good[:n])); err == nil {
			t.Fatalf("a cut at %d bytes of %d was read", n, len(good))
		}
	}
	var w binary.Writer
	w.Uvarint(2)  // steps
	w.Uvarint(50) // limit
	w.Uvarint(2)  // next
	w.Uvarint(0)  // clock
	w.Uvarint(2)  // two in the grave: both the same ID
	w.Uvarint(1)
	w.Uvarint(2)
	w.Uvarint(1)
	w.Uvarint(2)
	w.Uvarint(0)
	if _, err := ids.Decode[testID](binary.NewReader(w.B)); err == nil {
		t.Fatal("an ID in two places was read")
	}
	w.B = w.B[:0]
	w.Uvarint(2)
	w.Uvarint(50)
	w.Uvarint(2)
	w.Uvarint(0)
	w.Uvarint(1<<40 | 1) // a count that the bytes left cannot hold
	if _, err := ids.Decode[testID](binary.NewReader(w.B)); err == nil {
		t.Fatal("a count that is too large was read")
	}
}
