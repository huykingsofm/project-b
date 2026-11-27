package eventlog_test

import (
	"bufio"
	"math/rand/v2"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

var killEngines = []sim.StateID{
	sim.StateID(1), sim.StateID(2), sim.StateID(11),
}

// lastTick is the tick of the last record of an engine's log, or 0 when it has none.
func lastTick(t testing.TB, dir string, engine sim.StateID) uint64 {
	t.Helper()
	r, err := eventlog.ReadFileLog(dir, engine, 0, toyCodecs())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var last uint64
	for r.Next() {
		last = r.Record().Tick
	}
	if r.Err() != nil {
		t.Fatalf("%v: %v", engine, r.Err())
	}
	return last
}

// TestKillChild is the process that TestAProcessThatIsKilled... starts and kills: it is not a test.
// It opens the log in the directory it was given, goes on from the tick after the end of each
// engine's log, and records ticks of as many records as it is told (REPLAYLOG_KILL_RECORDS) on one
// goroutine for each engine as fast as it can, until it is killed.
func TestKillChild(t *testing.T) {
	dir := os.Getenv("REPLAYLOG_KILL_DIR")
	if dir == "" {
		t.Skip("started only by TestAProcessThatIsKilledLeavesALogThatReadsAndCanBeContinued")
	}
	records, err := strconv.Atoi(os.Getenv("REPLAYLOG_KILL_RECORDS"))
	if err != nil {
		t.Fatal(err)
	}
	log := eventlog.NewFileLog(
		dir,
		eventlog.FileLogOptions{
			Seed: 1, SegmentBytes: 8 << 20, SyncEvery: 10 * time.Millisecond, Backlog: 8,
		},
	)
	var wg sync.WaitGroup
	var ready sync.Once
	var started sync.WaitGroup
	started.Add(len(killEngines))
	for _, e := range killEngines {
		first := lastTick(t, dir, e) + 1
		wg.Go(func() {
			r := newTickRecorder(log, e, records)
			for tick := first; ; tick++ {
				r.tick(tick)
				if tick == first+20 {
					started.Done()
				}
			}
		})
	}
	started.Wait()
	ready.Do(func() { os.Stdout.WriteString("ready\n") })
	wg.Wait() // never: the parent kills the process
}

// A process that is killed while it writes leaves a log that reads without an error, in which every
// tick has all its records (the one that was being written is not read at all), and that the next
// process goes on from: six times in the same directory, each time at another moment. A tick is a
// batch of one write, and a kill can stop a write between two pages of the kernel: the first
// version of the log kept the records that were written and this test found a tick with 43 records
// of 100. A batch is now all or nothing (see segmentlog).
func TestAProcessThatIsKilledLeavesALogThatReadsAndCanBeContinued(t *testing.T) {
	killRounds(t, 100)
}

func killRounds(t *testing.T, records int) {
	dir := t.TempDir()
	rng := rand.New(rand.NewPCG(7, 8))
	var previous [3]uint64
	for round := range 6 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestKillChild$")
		cmd.Env = append(
			os.Environ(),
			"REPLAYLOG_KILL_DIR="+dir,
			"REPLAYLOG_KILL_RECORDS="+strconv.Itoa(records),
		)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(out).ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "ready") {
			cmd.Process.Kill()
			t.Fatalf("round %d: the process did not say it was ready: %q, %v", round, line, err)
		}
		time.Sleep(time.Duration(rng.IntN(60)) * time.Millisecond) // a different moment each round
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		cmd.Wait() // "signal: killed"

		for i, e := range killEngines {
			r, err := eventlog.ReadFileLog(dir, e, 0, toyCodecs())
			if err != nil {
				t.Fatalf("round %d, %v: %v", round, e, err)
			}
			perTick := map[uint64]int{}
			var last uint64
			for r.Next() {
				rec := r.Record()
				if rec.Tick < last {
					t.Fatalf("round %d, %v: tick %d after tick %d", round, e, rec.Tick, last)
				}
				last = rec.Tick
				perTick[rec.Tick]++
			}
			if r.Err() != nil {
				t.Fatalf(
					"round %d, %v: the log of a killed process does not read: %v",
					round,
					e,
					r.Err(),
				)
			}
			r.Close()
			for tick, count := range perTick {
				if count != records {
					t.Fatalf(
						"round %d, %v: tick %d has %d records of %d: a tick is all or nothing",
						round,
						e,
						tick,
						count,
						records,
					)
				}
			}
			if last <= previous[i] {
				t.Fatalf(
					"round %d, %v: the log did not grow: last tick %d, %d before",
					round,
					e,
					last,
					previous[i],
				)
			}
			previous[i] = last
		}
	}

	// the log can be written to again by this process too, and reads as one log
	log := eventlog.NewFileLog(
		dir, eventlog.FileLogOptions{Seed: 1, SegmentBytes: 8 << 20, SyncEvery: time.Second,
			Backlog: 8},
	)
	engine := killEngines[0]
	r := newTickRecorder(log, engine, records)
	for tick := previous[0] + 1; tick <= previous[0]+10; tick++ {
		r.tick(tick)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if got := lastTick(t, dir, engine); got != previous[0]+10 {
		t.Fatalf("after continuing the log ends at tick %d, want %d", got, previous[0]+10)
	}
}
