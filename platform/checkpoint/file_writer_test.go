package checkpoint

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/replay"
	"github.com/huykingsofm/project-b/platform/sim"
)

const engineA, engineB sim.StateID = 1<<56 | 1, 1<<56 | 2

// bytesOf is an encode that gives the bytes.
func bytesOf(b []byte) func() ([]byte, error) { return func() ([]byte, error) { return b, nil } }

func TestTheCheckpointsOfferedAreWrittenToTheFilesOfTheirEnginesAndReadBack(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(dir, 8)
	w.Offer(engineA, 20, bytesOf([]byte("a20")))
	w.Offer(engineB, 3, bytesOf([]byte("b3")))
	w.Offer(engineA, 5, bytesOf([]byte("a5")))
	w.Close()
	if w.Written() != 3 || w.Dropped() != 0 || w.Failed() != 0 {
		t.Fatalf("written %d, dropped %d, failed %d", w.Written(), w.Dropped(), w.Failed())
	}
	ticks, err := Ticks(dir, engineA)
	if err != nil || !slices.Equal(ticks, []uint64{5, 20}) {
		t.Fatalf("the ticks of A are %v, %v: want them in rising order", ticks, err)
	}
	if ticks, _ := Ticks(dir, engineB); !slices.Equal(ticks, []uint64{3}) {
		t.Fatalf("the ticks of B are %v", ticks)
	}
	for _, c := range []struct {
		engine sim.StateID
		tick   uint64
		want   string
	}{{engineA, 20, "a20"}, {engineA, 5, "a5"}, {engineB, 3, "b3"}} {
		got, err := Read(dir, c.engine, c.tick)
		if err != nil || !bytes.Equal(got, []byte(c.want)) {
			t.Fatalf("%v at %d: %q, %v", c.engine, c.tick, got, err)
		}
	}
}

func TestAnEngineWithNoDirectoryHasNoCheckpoint(t *testing.T) {
	ticks, err := Ticks(t.TempDir(), engineA)
	if err != nil || len(ticks) != 0 {
		t.Fatalf("%v, %v", ticks, err)
	}
}

func TestATemporaryFileThatACrashLeftIsNotACheckpointAndIsWrittenOver(t *testing.T) {
	dir := t.TempDir()
	engineDir := replay.EngineDir(dir, engineA)
	if err := os.MkdirAll(engineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	half := Path(dir, engineA, 9) + tmpSuffix
	if err := os.WriteFile(half, []byte("half a checkpoint that is longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(engineDir, "not-a-checkpoint.snap")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if ticks, _ := Ticks(dir, engineA); len(ticks) != 0 {
		t.Fatalf("a temporary file or another file is read as a checkpoint: %v", ticks)
	}
	w := NewFileWriter(dir, 1)
	w.Offer(engineA, 9, bytesOf([]byte("whole")))
	w.Close()
	if got, err := Read(dir, engineA, 9); err != nil || string(got) != "whole" {
		t.Fatalf("%q, %v", got, err)
	}
	if _, err := os.Stat(half); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the temporary file is still there: %v", err)
	}
}

func TestACheckpointThatFindsTheQueueFullIsDroppedAndTheOthersAreWritten(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(dir, 1)
	started, release := make(chan struct{}), make(chan struct{})
	w.Offer(engineA, 1, func() ([]byte, error) {
		close(started)
		<-release // the writer is busy with this one
		return []byte("1"), nil
	})
	<-started
	w.Offer(engineA, 2, bytesOf([]byte("2"))) // waits in the queue
	w.Offer(engineA, 3, bytesOf([]byte("3"))) // finds it full
	close(release)
	w.Close()
	if w.Written() != 2 || w.Dropped() != 1 {
		t.Fatalf("written %d, dropped %d: want 2 and 1", w.Written(), w.Dropped())
	}
	if ticks, _ := Ticks(dir, engineA); !slices.Equal(ticks, []uint64{1, 2}) {
		t.Fatalf("the ticks are %v, want 1 and 2", ticks)
	}
}

func TestACheckpointThatCannotBeEncodedIsCountedAndLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(dir, 1)
	w.Offer(engineA, 4, func() ([]byte, error) { return nil, errors.New("cannot") })
	w.Close()
	if w.Failed() != 1 || w.Written() != 0 {
		t.Fatalf("failed %d, written %d", w.Failed(), w.Written())
	}
	if ticks, _ := Ticks(dir, engineA); len(ticks) != 0 {
		t.Fatalf("a checkpoint that failed has a file: %v", ticks)
	}
}

func TestACheckpointOfferedAfterCloseIsDroppedAndCloseCanBeCalledAgain(t *testing.T) {
	dir := t.TempDir()
	w := NewFileWriter(dir, 1)
	w.Close()
	w.Offer(engineA, 1, bytesOf([]byte("late")))
	w.Close()
	if w.Dropped() != 1 || w.Written() != 0 {
		t.Fatalf("dropped %d, written %d", w.Dropped(), w.Written())
	}
}

// benchmarkWriteFile measures writing the file of a checkpoint of the size given, synced to the
// disk: what the goroutine of the writer pays for each, besides encoding it.
func benchmarkWriteFile(b *testing.B, size int) {
	dir := b.TempDir()
	data := bytes.Repeat([]byte{7}, size)
	b.SetBytes(int64(size))
	tick := uint64(0)
	for b.Loop() {
		tick++
		if err := writeFile(dir, engineA, tick, data); err != nil {
			b.Fatal(err)
		}
	}
}

// The sizes of the checkpoints of a full zonelayer and of a world of 2,000 players (see
// docs/benchmark/checkpoint.md).
func BenchmarkWriteFileZoneLayer(b *testing.B) { benchmarkWriteFile(b, 8_300) }
func BenchmarkWriteFileWorld(b *testing.B)     { benchmarkWriteFile(b, 122_000) }
