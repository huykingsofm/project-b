package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
)

// removeEngineAction asks the cluster to remove an engine.
type removeEngineAction struct{ Engine sim.StateID }

func (removeEngineAction) ID() sim.ActionID { return 60011 }
func (removeEngineAction) Priority() int16  { return 0 }
func (removeEngineAction) Actor() any       { return nil }
func (a removeEngineAction) Apply(
	_ *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	r.RequestEngineRemoval(a.Engine)
}
func (a removeEngineAction) Clone() sim.Action[*toyState] { return a }

// recordedControl is an EngineControl that remembers what it was asked.
type recordedControl struct {
	mu  sync.Mutex
	ids []sim.StateID
}

func (c *recordedControl) RemoveEngine(id sim.StateID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, id)
}

// toysThatCanRemoveOneAnother is a cluster of the toy engines 1 to n, whose environment has a
// ClusterControl, and the log that it writes.
func toysThatCanRemoveOneAnother(
	t testing.TB, n int,
) (*Cluster, []*Engine[*toyState], *eventlog.MemLog) {
	t.Helper()
	env := newToyEnv(t)
	log := &eventlog.MemLog{}
	env.EventLog = log
	env.Control = NewClusterControl()
	c := newCluster(t, env)
	var engines []*Engine[*toyState]
	for id := sim.StateID(1); id <= sim.StateID(n); id++ {
		e := New(&toyState{id: id}, env, 1)
		engines = append(engines, e)
		c.Add(e)
	}
	return c, engines, log
}

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

func TestARequestToRemoveAnEngineIsHandedToTheControlWhenTheTickIsDone(t *testing.T) {
	env := newToyEnv(t)
	control := &recordedControl{}
	env.Control = control
	c := newCluster(t, env)
	a := New(&toyState{id: 1}, env, 1)
	c.Add(a)
	a.ScheduleAt(2, removeEngineAction{Engine: 7})
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if len(control.ids) != 0 {
		t.Fatalf("%v was asked before the tick that asks", control.ids)
	}
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if len(control.ids) != 1 || control.ids[0] != 7 {
		t.Fatalf("the control was asked %v, want engine 7 once", control.ids)
	}
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if len(control.ids) != 1 {
		t.Fatalf("the control was asked %v: a request is handed over once", control.ids)
	}
}

func TestAnEngineWithNoControlIgnoresARequestToRemoveAnEngine(t *testing.T) {
	c, engines := toyCluster(t, 1, 2) // the environment of a replay has no control
	engines[0].ScheduleAt(1, removeEngineAction{Engine: 2})
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find(2); !ok {
		t.Fatal("an engine was removed in an environment that has no control")
	}
}

func TestAStepRemovesTheEngineThatAnActionAskedForWhenTheStepEnds(t *testing.T) {
	c, engines, log := toysThatCanRemoveOneAnother(t, 3)
	a, b := engines[0], engines[1]
	a.ScheduleAt(1, removeEngineAction{Engine: 2})
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find(2); ok || !b.Removed() {
		t.Fatalf("engine 2 is in the cluster (%v) or not removed (%v)", ok, b.Removed())
	}
	last, alone := lastRecordOf(log, 2)
	if _, isRetired := last.Body.(eventlog.Retired); !isRetired || alone != 1 || last.Tick != 2 {
		t.Fatalf("the last of its log is %+v (%d at its tick), want Retired alone at the tick 2",
			last, alone)
	}
	before := len(log.ByEngine()[2])
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if got := len(log.ByEngine()[2]); got != before {
		t.Fatalf("a removed engine wrote %d records after its removal", got-before)
	}
}

func TestAMessageForAnEngineThatWasRemovedComesBackAsRetired(t *testing.T) {
	c, engines, _ := toysThatCanRemoveOneAnother(t, 3)
	engines[0].ScheduleAt(1, removeEngineAction{Engine: 2})
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	reason, ok := c.Env.Router.Deliver(toMessage(2, 1))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message for the engine that was removed: %v, %v", reason, ok)
	}
}

func TestTheConfigurationEngineIsNeverRemoved(t *testing.T) {
	c, _, _ := toysThatCanRemoveOneAnother(t, 1)
	if c.Remove(sim.ConfigStateID) {
		t.Fatal("Remove took the configuration engine out")
	}
	c.RemoveEngine(sim.ConfigStateID)
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find(sim.ConfigStateID); !ok || c.ConfigEngine().Removed() {
		t.Fatal("a request removed the configuration engine")
	}
}

func TestARequestForAnEngineThatTheClusterDoesNotHaveChangesNothing(t *testing.T) {
	c, _, _ := toysThatCanRemoveOneAnother(t, 2)
	c.RemoveEngine(99)
	if err := c.Step(); err != nil {
		t.Fatal(err)
	}
	if got := len(c.Engines()); got != 3 { // the configuration engine and the two
		t.Fatalf("the cluster has %d engines, want 3", got)
	}
}

// runFor runs the cluster on its own goroutines until the condition holds or a second has gone, and
// returns after Run has returned.
func runUntil(t *testing.T, c *Cluster, done func() bool) {
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
	c, engines, log := toysThatCanRemoveOneAnother(t, 3)
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
	c, _, log := toysThatCanRemoveOneAnother(t, n)
	var wg sync.WaitGroup
	for id := sim.StateID(2); id <= n; id += 2 {
		wg.Go(func() {
			time.Sleep(time.Duration(id) * time.Millisecond)
			c.RemoveEngine(id)
			c.RemoveEngine(id) // a second request changes nothing
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
