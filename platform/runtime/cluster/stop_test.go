package cluster_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
)

// runFor runs the cluster on its goroutines for the time given, and returns when it has stopped.
func runFor(t *testing.T, c *cluster.Cluster, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if err := c.Run(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestEnginesThatRanOnTheirGoroutinesAndStoppedAreWhereStepsPutThemAtTheSameTick(t *testing.T) {
	const engines, lastTick = 3, 60
	schedule := func(toys []*engine.Engine[*testkit.ToyState]) {
		for i, e := range toys {
			for tick := uint64(1); tick <= lastTick; tick++ {
				e.ScheduleAt(tick, testkit.ToyAction{N: int64(tick) * int64(i+1)})
			}
		}
	}
	c, toys := testkit.NewToyCluster(t, engines)
	schedule(toys)
	runFor(t, c, 40*time.Millisecond)

	_, again := testkit.NewToyCluster(t, engines)
	schedule(again)
	for i, e := range toys {
		want, err := e.EncodeCheckpoint()
		if err != nil {
			t.Fatal(err)
		}
		for again[i].Tick() < e.Tick() {
			if err := again[i].Step(); err != nil {
				t.Fatal(err)
			}
		}
		got, err := again[i].EncodeCheckpoint()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("the engine %v stopped at the tick %d: a Step to it gives another state",
				e.StateID(), e.Tick())
		}
	}
}

func TestAStopLosesNoMessageThatWasOnItsWay(t *testing.T) {
	const sent = 400
	for _, runMillis := range []int{5, 12, 25, 50} {
		t.Run(fmt.Sprintf("%dms", runMillis), func(t *testing.T) {
			c, toys := testkit.NewToyCluster(t, 2)
			from, to := toys[0], toys[1]
			for tick := uint64(1); tick <= sent; tick++ {
				from.ScheduleAt(tick, testkit.ForwardAction{To: to.StateID(), N: 1})
			}
			runFor(t, c, time.Duration(runMillis)*time.Millisecond)
			if from.Tick() >= sent {
				t.Skip("the cluster ran all of it before it stopped: nothing was on its way")
			}
			// the process that stopped is gone: what is left is what the engines would write
			again, _ := testkit.NewToyCluster(t, 0)
			var restored []*engine.Engine[*testkit.ToyState]
			for _, e := range toys {
				data, err := e.EncodeCheckpoint()
				if err != nil {
					t.Fatal(err)
				}
				handle, err := again.AddEngineByCheckpoint(data)
				if err != nil {
					t.Fatal(err)
				}
				restored = append(restored, handle.(*engine.Engine[*testkit.ToyState]))
			}
			for range 3000 {
				if err := stepping.Step(again); err != nil {
					t.Fatal(err)
				}
			}
			if got := restored[1].State().Total; got != sent {
				t.Fatalf("the receiver took %d messages, want %d: every one once", got, sent)
			}
		})
	}
}
