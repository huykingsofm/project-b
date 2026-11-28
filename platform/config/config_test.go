package config_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/huykingsofm/project-b/platform/config"
)

// The tests declare variables of their own, with names that begin with "test.", so they do not
// depend on what the packages of the game declare. The declarations are made before the first
// snapshot is built, as every variable must be.
var (
	testCount = config.NewVar("test.count", 3, config.Range(1, 10))
	testRatio = config.NewVar("test.ratio", 0.5, config.Range(0.0, 1.0))
	testOn    = config.NewVar("test.on", false)
	testName  = config.NewVar("test.name", "x")
	testKeys  = config.NewVar[[]string]("test.keys", nil)
	testOther = config.NewVar("test.other", 1, config.Min(0))
	testPlain = config.NewVar(
		"test.plain", "x", config.Parse(func(text string) (string, error) { return text, nil }),
	)
)

func TestVariablesOfAnyKindAreParsedFromTheTextOfAJSONValue(t *testing.T) {
	d, err := config.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if testCount.Get(d) != 3 || testRatio.Get(d) != 0.5 || testOn.Get(d) ||
		testName.Get(d) != "x" || len(testKeys.Get(d)) != 0 {
		t.Fatal("a variable without an override is not at its default")
	}

	s, err := config.Build(map[string]string{
		"test.count": "7", "test.ratio": "0.25", "test.on": "true", "test.name": `"abc"`,
		"test.keys": `["a","b"]`,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if testCount.Get(s) != 7 || testRatio.Get(s) != 0.25 || !testOn.Get(s) ||
		testName.Get(s) != "abc" || !slices.Equal(testKeys.Get(s), []string{"a", "b"}) {
		t.Fatalf(
			"values = %v %v %v %v %v",
			testCount.Get(s),
			testRatio.Get(s),
			testOn.Get(s),
			testName.Get(s),
			testKeys.Get(s),
		)
	}
}

func TestABadOverrideRefusesTheWholeDocumentAndNamesEachOne(t *testing.T) {
	for _, text := range []string{`"seven"`, "11", "3 4", "", "2.5"} {
		snap, err := config.Build(map[string]string{"test.count": text, "test.other": "5"})
		if err == nil || snap != nil || !strings.Contains(err.Error(), "test.count") {
			t.Errorf(
				"override %q: snapshot %v, error %v; want no snapshot and an error naming "+
					"test.count",
				text,
				snap,
				err,
			)
		}
	}

	_, err := config.Build(map[string]string{
		"test.count": "99", "test.name": "5", "test.other": "-1",
	})
	if err == nil {
		t.Fatal("no error")
	}
	for _, name := range []string{"test.count", "test.name", "test.other"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestUnknownNamesAreIgnored(t *testing.T) {
	s, err := config.Build(map[string]string{"zeta": "1", "test.count": "4", "alpha": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if testCount.Get(s) != 4 {
		t.Fatalf("count %d", testCount.Get(s))
	}
}

func TestAnOptionCanReplaceHowTheTextIsRead(t *testing.T) {
	s, err := config.Build(map[string]string{"test.plain": "plain text"})
	if err != nil || testPlain.Get(s) != "plain text" {
		t.Fatalf("plain = %q, %v", testPlain.Get(s), err)
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
	mustPanic(
		"a default outside its range", func() {
			config.NewVar("test.range", 99, config.Range(1,
				10))
		},
	)
	mustPanic("a name declared twice", func() { config.NewVar("test.count", 1) })
	config.Defaults() // the first snapshot
	mustPanic("a declaration after the first snapshot", func() { config.NewVar("test.late", 1) })
}

func TestGetOnANilSnapshotGivesTheDefault(t *testing.T) {
	if testCount.Get(nil) != 3 {
		t.Fatal("not the default")
	}
}
