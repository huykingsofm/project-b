package engine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

// removeEngineAction asks the coordinator to end an engine.
type removeEngineAction struct{ Engine sim.StateID }

func (removeEngineAction) ID() sim.ActionID { return 60011 }
func (removeEngineAction) Priority() int16  { return 0 }
func (removeEngineAction) Actor() any       { return nil }
func (a removeEngineAction) Apply(
	_ *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	r.RequestRetireEngine(onfail.Drop{}, a.Engine)
}
func (a removeEngineAction) Clone() sim.Action[*toyState] { return a }

// lastRecordOf is the last record that the engine wrote, and how many it wrote at its tick.
func lastRecordOf(log *eventlog.MemLog, id sim.StateID) (eventlog.Record, int) {
	records := log.ByEngine()[id]
	if len(records) == 0 {
		return eventlog.Record{}, 0
	}
	last := records[len(records)-1]
	n := 0
	for _, r := range records {
		if r.Tick == last.Tick {
			n++
		}
	}
	return last, n
}

func TestAStepRemovesTheEngineThatAnActionAskedForWhenTheStepEndsAndItsLogDependsOnTheCoordinator(
	t *testing.T,
) {
	c, engines, log := toysWithALog(t, 3)
	a, b := engines[0], engines[1]
	a.ScheduleAt(1, removeEngineAction{Engine: 2})
	steps(t, c, 1) // the engine asks
	if _, ok := c.Find(2); !ok || b.IsClosed() {
		t.Fatal("engine 2 was removed before the coordinator took the request")
	}
	steps(t, c, 1) // the coordinator takes it, and the engine ends when the step ends
	if _, ok := c.Find(2); ok || !b.IsClosed() {
		t.Fatalf("engine 2 is in the cluster (%v) or not removed (%v)", ok, b.IsClosed())
	}
	last, alone := lastRecordOf(log, 2)
	retired, isRetired := last.Body.(eventlog.Retired)
	if !isRetired || alone != 1 || last.Tick != 3 {
		t.Fatalf("the last of its log is %+v (%d at its tick), want Retired alone at the tick 3",
			last, alone)
	}
	if retired.By.Source != sim.CoordinatorStateID || retired.By.Tick != c.Coordinator().Tick() {
		t.Fatalf("it was removed by %+v, want the tick %d of the coordinator",
			retired.By, c.Coordinator().Tick())
	}
	before := len(log.ByEngine()[2])
	steps(t, c, 1)
	if got := len(log.ByEngine()[2]); got != before {
		t.Fatalf("a removed engine wrote %d records after its removal", got-before)
	}
}

func TestAMessageForAnEngineThatWasRemovedComesBackAsRetired(t *testing.T) {
	c, engines, _ := toysWithALog(t, 3)
	engines[0].ScheduleAt(1, removeEngineAction{Engine: 2})
	steps(t, c, 2)
	reason, ok := c.Env.Router.Deliver(toMessage(2, 1))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message for the engine that was removed: %v, %v", reason, ok)
	}
}

func TestTheConfigurationEngineAndTheCoordinatorAreNeverRemoved(t *testing.T) {
	c, _, _ := toysWithALog(t, 1)
	for _, id := range []sim.StateID{sim.ConfigStateID, sim.CoordinatorStateID} {
		if !c.Coordinator().Send(sim.RetireEngineCommand{Target: id}) {
			t.Fatal("the command was refused")
		}
	}
	steps(t, c, 2)
	for _, id := range []sim.StateID{sim.ConfigStateID, sim.CoordinatorStateID} {
		if _, ok := c.Find(id); !ok {
			t.Fatalf("a request removed the engine %v", id)
		}
	}
	if c.ConfigEngine().IsClosed() || c.Coordinator().IsClosed() {
		t.Fatal("a request closed an engine that every cluster has")
	}
}

func TestARequestForAnEngineThatTheClusterDoesNotHaveChangesNothing(t *testing.T) {
	c, _, _ := toysWithALog(t, 2)
	if !c.Coordinator().Send(sim.RetireEngineCommand{Target: 99}) {
		t.Fatal("the command was refused")
	}
	steps(t, c, 2)
	if got := len(c.Engines()); got != 4 { // the configuration engine, the coordinator, 1 and 2
		t.Fatalf("the cluster has %d engines, want 4", got)
	}
}

// runUntil runs the cluster on its own goroutines until the condition holds or two seconds have
// gone, and returns after Run has returned.
func runUntil(t *testing.T, c *engine.Cluster, done func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(finished) }()
	deadline := time.After(2 * time.Second)
	for !done() {
		select {
		case <-deadline:
			cancel()
			<-finished
			t.Fatal("the condition did not hold in two seconds")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestAnEngineThatRunsOnItsOwnEndsBetweenTwoTicksWhenAnActionAsksForItsRemoval(t *testing.T) {
	c, engines, log := toysWithALog(t, 3)
	engines[0].ScheduleAt(5, removeEngineAction{Engine: 2})
	runUntil(t, c, func() bool { _, ok := c.Find(2); return !ok })
	last, alone := lastRecordOf(log, 2)
	if _, isRetired := last.Body.(eventlog.Retired); !isRetired || alone != 1 {
		t.Fatalf("the last of its log is %+v (%d at its tick), want Retired alone", last, alone)
	}
	retired := 0
	for _, r := range log.ByEngine()[2] {
		if _, isRetired := r.Body.(eventlog.Retired); isRetired {
			retired++
		}
	}
	if retired != 1 {
		t.Fatalf("%d records of its removal, want 1", retired)
	}
	if _, ok := c.Find(1); !ok {
		t.Fatal("the engine that asked was removed too")
	}
}

func TestEnginesAskedToBeRemovedFromManyGoroutinesWhileOthersRunAreAllRemoved(t *testing.T) {
	const n = 12
	c, _, log := toysWithALog(t, n)
	var wg sync.WaitGroup
	for id := sim.StateID(2); id <= n; id += 2 {
		wg.Go(func() {
			time.Sleep(time.Duration(id) * time.Millisecond)
			c.Coordinator().Send(sim.RetireEngineCommand{Target: id})
			c.Coordinator().Send(sim.RetireEngineCommand{Target: id}) // a second changes nothing
		})
	}
	reader := make(chan struct{})
	go func() { // the list is read while it changes
		defer close(reader)
		for range 200 {
			for _, e := range c.Engines() {
				c.Find(e.StateID())
			}
			time.Sleep(100 * time.Microsecond)
		}
	}()
	runUntil(t, c, func() bool {
		for id := sim.StateID(2); id <= n; id += 2 {
			if _, ok := c.Find(id); ok {
				return false
			}
		}
		return true
	})
	wg.Wait()
	<-reader
	for id := sim.StateID(1); id <= n; id++ {
		_, found := c.Find(id)
		if found != (id%2 == 1) {
			t.Fatalf("engine %d: in the cluster %v", id, found)
		}
		if id%2 == 0 {
			if _, isRetired := lastBody(log, id).(eventlog.Retired); !isRetired {
				t.Fatalf("the log of engine %d does not end with its removal", id)
			}
		}
	}
}

func lastBody(log *eventlog.MemLog, id sim.StateID) eventlog.Body {
	records := log.ByEngine()[id]
	return records[len(records)-1].Body
}
