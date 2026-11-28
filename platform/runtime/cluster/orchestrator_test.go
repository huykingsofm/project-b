package cluster_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/sim"
)

func TestTheOrchestratorCountsUpFromOneForEachTypeAndStoresWhatItGave(t *testing.T) {
	dir := t.TempDir()
	o, err := cluster.NewOrchestrator(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []sim.StateID{
		sim.NewStateID(1, 1), sim.NewStateID(1, 2), sim.NewStateID(2, 1),
	} {
		got, err := o.NextStateID(want.Type())
		if err != nil || got != want {
			t.Fatalf("the next ID is %v, %v, want %v", got, err, want)
		}
	}
	marks, found, err := cluster.ReadStateIDMarks(dir)
	if err != nil || !found || marks[1] != 2 || marks[2] != 1 || len(marks) != 2 {
		t.Fatalf("the file holds %v, found %v, %v, want 2 for the type 1 and 1 for the type 2",
			marks, found, err)
	}
	again, err := cluster.NewOrchestrator(dir, nil) // a cluster after it goes on from the file
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := again.NextStateID(1); got != sim.NewStateID(1, 3) {
		t.Fatalf("a number was given again: %v", got)
	}
}

func TestAnOrchestratorThatStoresNothingStillCounts(t *testing.T) {
	o, err := cluster.NewOrchestrator("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := o.NextStateID(3); got != sim.NewStateID(3, 1) {
		t.Fatalf("got %v", got)
	}
	if o.HighestStateIDs()[3] != 1 {
		t.Fatalf("the highest is %v", o.HighestStateIDs())
	}
}

func TestAFileOfTheIDsThatCannotBeReadIsAnErrorAndNotGuessedAt(t *testing.T) {
	dir := t.TempDir()
	if marks, found, err := cluster.ReadStateIDMarks(dir); err != nil || found || len(marks) != 0 {
		t.Fatalf("a directory with no file: %v, %v, %v", marks, found, err)
	}
	for name, data := range map[string][]byte{
		"cut":           {1, 2, 1},
		"another data":  {9, 0},
		"bytes after":   {1, 0, 7},
		"a type twice":  {1, 2, 1, 1, 1, 2},
		"more than due": {1, 200},
	} {
		file := filepath.Join(dir, cluster.StateIDMarksFile)
		if err := os.WriteFile(file, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := cluster.ReadStateIDMarks(dir); err == nil {
			t.Fatalf("%s: a damaged file was read", name)
		}
		if _, err := cluster.NewOrchestrator(dir, nil); err == nil {
			t.Fatalf("%s: an orchestrator was made from a damaged file", name)
		}
	}
}
