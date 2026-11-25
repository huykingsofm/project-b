package atomicfile_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/shared/atomicfile"
)

func TestWriteMakesTheDirectoryAndReplacesAFileWholeAndLeavesNoTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "file")
	for _, want := range []string{"first and long", "second"} {
		if err := atomicfile.Write(path, []byte(want)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("the file holds %q (%v), want %q", got, err, want)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("the directory holds %d entries (%v), want only the file", len(entries), err)
	}
}

func TestWriteOverATemporaryFileThatACrashLeft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path+".tmp", []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(path, []byte("whole")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "whole" {
		t.Fatalf("the file holds %q", got)
	}
}

func TestSyncDirOfADirectoryThatIsNotThereIsAnError(t *testing.T) {
	if err := atomicfile.SyncDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.SyncDir(filepath.Join(t.TempDir(), "nothing")); err == nil {
		t.Fatal("a directory that is not there was synced")
	}
}

func TestStampedPutsTheTimeBeforeTheExtensionAndKeepsTwoFilesOfOneNameApart(t *testing.T) {
	at := time.Date(2026, 11, 25, 8, 30, 15, 42, time.UTC)
	cases := map[string]string{
		"seg-0001.pbsl": "seg-0001.20261125T083015000000042Z.pbsl",
		"ckpt-7.snap":   "ckpt-7.20261125T083015000000042Z.snap",
		"noextension":   "noextension.20261125T083015000000042Z",
	}
	for name, want := range cases {
		if got := atomicfile.Stamped(name, at); got != want {
			t.Errorf("Stamped(%q) = %q, want %q", name, got, want)
		}
	}
	zone := time.FixedZone("x", 7*3600)
	if a, b := atomicfile.Stamped("f.x", at), atomicfile.Stamped("f.x", at.In(zone)); a != b {
		t.Errorf("the same instant has two names: %q and %q", a, b)
	}
	if a, b := atomicfile.Stamped("f.x", at), atomicfile.Stamped("f.x", at.Add(1)); a == b {
		t.Errorf("two instants have one name: %q", a)
	}
}
