package config

import (
	"slices"
	"strings"
	"testing"
)

// The tests declare their variables in a registry of their own, so they do not depend on what
// the packages of the game declare.

func TestVariablesOfAnyKindAreParsedFromTheTextOfAJSONValue(t *testing.T) {
	r := newRegistry()
	count := declare(r, "count", 3, []Option[int]{Range(1, 10)})
	ratio := declare(r, "ratio", 0.5, []Option[float64]{Range(0.0, 1.0)})
	on := declare[bool](r, "on", false, nil)
	name := declare[string](r, "name", "x", nil)
	keys := declare[[]string](r, "keys", nil, nil)

	d, _, err := r.build(0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count.Get(d) != 3 || ratio.Get(d) != 0.5 || on.Get(d) || name.Get(d) != "x" || len(keys.Get(d)) != 0 {
		t.Fatal("a variable without an override is not at its default")
	}

	s, unknown, err := r.build(1, map[string]string{
		"count": "7", "ratio": "0.25", "on": "true", "name": `"abc"`, "keys": `["a","b"]`,
	})
	if err != nil || len(unknown) != 0 {
		t.Fatalf("build: %v, unknown %v", err, unknown)
	}
	if count.Get(s) != 7 || ratio.Get(s) != 0.25 || !on.Get(s) || name.Get(s) != "abc" || !slices.Equal(keys.Get(s), []string{"a", "b"}) {
		t.Fatalf("values = %v %v %v %v %v", count.Get(s), ratio.Get(s), on.Get(s), name.Get(s), keys.Get(s))
	}
	if s.Version() != 1 {
		t.Fatalf("version = %d", s.Version())
	}
}

func TestABadOverrideRefusesTheWholeDocumentAndNamesEachOne(t *testing.T) {
	r := newRegistry()
	declare(r, "count", 3, []Option[int]{Range(1, 10)})
	declare[string](r, "name", "x", nil)
	declare(r, "other", 1, []Option[int]{Min(0)})

	for _, text := range []string{`"seven"`, "11", "3 4", "", "2.5"} {
		snap, _, err := r.build(1, map[string]string{"count": text, "other": "5"})
		if err == nil || snap != nil || !strings.Contains(err.Error(), "count") {
			t.Errorf("override %q: snapshot %v, error %v; want no snapshot and an error naming count", text, snap, err)
		}
	}

	_, _, err := r.build(1, map[string]string{"count": "99", "name": "5", "other": "-1"})
	if err == nil {
		t.Fatal("no error")
	}
	for _, name := range []string{"count", "name", "other"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestUnknownNamesAreReturnedInOrderAndIgnored(t *testing.T) {
	r := newRegistry()
	count := declare(r, "count", 3, []Option[int](nil))
	s, unknown, err := r.build(1, map[string]string{"zeta": "1", "count": "4", "alpha": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(unknown, []string{"alpha", "zeta"}) || count.Get(s) != 4 {
		t.Fatalf("unknown %v, count %d", unknown, count.Get(s))
	}
}

func TestAnOptionCanReplaceHowTheTextIsRead(t *testing.T) {
	r := newRegistry()
	name := declare(r, "name", "x", []Option[string]{Parse(func(text string) (string, error) { return text, nil })})
	s, _, err := r.build(1, map[string]string{"name": "plain text"})
	if err != nil || name.Get(s) != "plain text" {
		t.Fatalf("name = %q, %v", name.Get(s), err)
	}
}

func TestADefaultThatBreaksALimitAndALateDeclarationPanic(t *testing.T) {
	mustPanic := func(what string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", what)
			}
		}()
		f()
	}
	r := newRegistry()
	mustPanic("a default outside its range", func() { declare(r, "a", 99, []Option[int]{Range(1, 10)}) })
	declare(r, "b", 1, []Option[int](nil))
	mustPanic("a name declared twice", func() { declare(r, "b", 1, []Option[int](nil)) })
	r.build(0, nil)
	mustPanic("a declaration after the first snapshot", func() { declare(r, "c", 1, []Option[int](nil)) })
}

func TestGetOnANilSnapshotGivesTheDefault(t *testing.T) {
	r := newRegistry()
	v := declare(r, "count", 3, []Option[int](nil))
	if v.Get(nil) != 3 {
		t.Fatal("not the default")
	}
}
