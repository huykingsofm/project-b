package segmentlog

import (
	"path/filepath"
	"testing"
)

func TestSyncDirWorksForADirectoryAndFailsForOneThatIsNotThere(t *testing.T) {
	if err := syncDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := syncDir(filepath.Join(t.TempDir(), "nothing")); err == nil {
		t.Fatal("the sync of a directory that does not exist did not fail")
	}
}
