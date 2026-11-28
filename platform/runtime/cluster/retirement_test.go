package cluster_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
)

// retireEngineAction asks the cluster to retire an engine.
type retireEngineAction struct{ Engine sim.StateID }

func (retireEngineAction) ID() sim.ActionID { return 60011 }
func (retireEngineAction) Priority() int16  { return 0 }
func (retireEngineAction) Actor() any       { return nil }
func (a retireEngineAction) Apply(
	_ *sim.Context, _ *testkit.ToyState, r *sim.ActionRegistry[*testkit.ToyState],
	_ *sim.Plan[*testkit.ToyState],
) {
	r.RequestRetireEngine(a.Engine, nil)
}
func (a retireEngineAction) Clone() sim.Action[*testkit.ToyState] { return a }

// toysThatCanRetireOneAnother is a cluster of the toy engines 1 to n, which hand their requests to
// the cluster, and the log that it writes.
func toysThatCanRetireOneAnother(
	t testing.TB, n int,
) (*cluster.Cluster, []*engine.Engine[*testkit.ToyState], *eventlog.MemLog) {
	t.Helper()
	env := testkit.NewToyEnv(t)
	log := &eventlog.MemLog{}
	env.EventLog = log
	c := testkit.NewCluster(t, env)
	return c, testkit.AddToys(t, c, n), log
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

func TestARequestToRetireAnEngineIsPublishedToTheRouterWhenTheTickIsDone(t *testing.T) {
	env := testkit.NewToyEnv(t)
	a := engine.NewEngine(&testkit.ToyState{}, 1, env)
	a.ScheduleAt(2, retireEngineAction{Engine: 7})
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if got := env.DirectiveRouter.TakeRequests(); len(got) != 0 {
		t.Fatalf("%v was asked before the tick that asks", got)
	}
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	got := env.DirectiveRouter.TakeRequests()
	if len(got) != 1 || got[0].From != 1 || got[0].Request.Target != 7 ||
		got[0].Request.Tick != 2 || got[0].Request.ID != 1 {
		t.Fatalf("the router holds %+v, want the request 1 of engine 1 at its tick 2 for engine 7",
			got)
	}
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if got := env.DirectiveRouter.TakeRequests(); len(got) != 0 {
		t.Fatalf("%v: a request is published once while the engine runs", got)
	}
}

func TestAStepRetiresTheEngineThatAnActionAskedForWhenTheStepEnds(t *testing.T) {
	c, engines, log := toysThatCanRetireOneAnother(t, 3)
	a, b := engines[0], engines[1]
	a.ScheduleAt(1, retireEngineAction{Engine: 2})
	steps(t, c, 2) // the request is published at the end of the first step, served in the second
	_, retired := b.IsRetired()
	if _, ok := c.Find(2); ok || !retired {
		t.Fatalf("engine 2 is in the cluster (%v) or did not retire (%v)", ok, retired)
	}
	last, alone := lastRecordOf(log, 2)
	if _, isRetired := last.Body.(eventlog.Retired); !isRetired || alone != 1 || last.Tick != 3 {
		t.Fatalf("the last of its log is %+v (%d at its tick), want Retired alone at the tick 3",
			last, alone)
	}
	before := len(log.ByEngine()[2])
	if err := stepping.Step(c); err != nil {
		t.Fatal(err)
	}
	if got := len(log.ByEngine()[2]); got != before {
		t.Fatalf("a retired engine wrote %d records after its retirement", got-before)
	}
}

func TestAMessageForAnEngineThatWasRetiredComesBackAsRetired(t *testing.T) {
	c, engines, _ := toysThatCanRetireOneAnother(t, 3)
	engines[0].ScheduleAt(1, retireEngineAction{Engine: 2})
	steps(t, c, 3)
	reason, ok := c.Env.Router.Deliver(testkit.ToyMessage(2, 1))
	if ok || reason != sim.DeliveryFailureRetired {
		t.Fatalf("a message for the engine that was retired: %v, %v", reason, ok)
	}
}

func TestTheCoordinatorEngineIsNeverRetired(t *testing.T) {
	c, engines, log := toysThatCanRetireOneAnother(t, 1)
	engines[0].ScheduleAt(1, retireEngineAction{Engine: sim.CoordinatorStateID})
	steps(t, c, 3)
	if _, retired := c.CoordinatorEngine().IsRetired(); retired {
		t.Fatal("the coordinator engine retired")
	}
	if _, ok := c.Find(sim.CoordinatorStateID); !ok {
		t.Fatal("a request retired the coordinator engine")
	}
	for _, r := range log.ByEngine()[1] {
		if d, ok := r.Body.(eventlog.Directive); ok && d.Response.Failure == "" {
			t.Fatalf("the answer %+v is not a failure", d.Response)
		}
	}
}

func TestARequestForAnEngineThatTheClusterDoesNotHaveIsAnsweredWithAFailure(t *testing.T) {
	c, engines, log := toysThatCanRetireOneAnother(t, 2)
	engines[0].ScheduleAt(1, retireEngineAction{Engine: 99})
	steps(t, c, 3)
	if got := len(c.Engines()); got != 3 { // the coordinator engine and the two
		t.Fatalf("the cluster has %d engines, want 3", got)
	}
	answers := 0
	for _, r := range log.ByEngine()[1] {
		if d, ok := r.Body.(eventlog.Directive); ok {
			answers++
			if d.Response.Failure == "" {
				t.Fatalf("the answer %+v is not a failure", d.Response)
			}
		}
	}
	if answers != 1 {
		t.Fatalf("%d answers, want 1", answers)
	}
}

func TestTheAnswerToARequestToRetireAnotherEngineCarriesTheTickOfItsRetiredAndDependsOnIt(
	t *testing.T,
) {
	c, engines, log := toysThatCanRetireOneAnother(t, 3)
	engines[0].ScheduleAt(2, retireEngineAction{Engine: 2})
	steps(t, c, 5)
	if _, ok := c.Find(2); ok {
		t.Fatal("the engine 2 is still in the cluster")
	}
	last, alone := lastRecordOf(log, 2)
	retired, isRetired := last.Body.(eventlog.Retired)
	if !isRetired || alone != 1 || retired.By != 1 || retired.ByTick != 2 {
		t.Fatalf("the last of its log is %+v, want Retired alone, asked by engine 1 at its tick 2",
			last)
	}
	if dep, ok := last.Dependency(); !ok || dep != (sim.StateDependency{Source: 1, Tick: 2}) {
		t.Fatalf("Retired depends on %+v, %v, want the tick 2 of the engine 1", dep, ok)
	}
	for _, r := range log.ByEngine()[1] {
		d, ok := r.Body.(eventlog.Directive)
		if !ok {
			continue
		}
		dep, depends := r.Dependency()
		if d.Response.Engine != 2 || d.Response.Tick != last.Tick || !depends ||
			dep != (sim.StateDependency{Source: 2, Tick: last.Tick}) {
			t.Fatalf("the answer %+v depends on %+v, %v, want the Retired at the tick %d",
				d.Response, dep, depends, last.Tick)
		}
	}
}

func TestAnEngineThatAsksForItselfToEndGetsNoAnswerAndItsRetiredHasNoOneThatAsked(t *testing.T) {
	c, engines, log := toysThatCanRetireOneAnother(t, 2)
	engines[1].ScheduleAt(1, retireEngineAction{Engine: 2})
	steps(t, c, 4)
	if _, ok := c.Find(2); ok {
		t.Fatal("the engine 2 is still in the cluster")
	}
	last, _ := lastRecordOf(log, 2)
	if retired, ok := last.Body.(eventlog.Retired); !ok || retired != (eventlog.Retired{}) {
		t.Fatalf("the last of its log is %+v, want an empty Retired", last)
	}
	for _, r := range log.ByEngine()[2] {
		if _, answered := r.Body.(eventlog.Directive); answered {
			t.Fatalf("an engine that ended got an answer: %+v", r)
		}
	}
}

// runFor runs the cluster on its own goroutines until the condition holds or a second has gone, and
// returns after Run has returned.
func runUntil(t *testing.T, c *cluster.Cluster, done func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		if err := c.Run(ctx, time.Millisecond); err != nil {
			t.Error(err)
		}
		close(finished)
	}()
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

func TestAnEngineThatRunsOnItsOwnEndsBetweenTwoTicksWhenAnActionAsksForItsRetirement(t *testing.T) {
	c, engines, log := toysThatCanRetireOneAnother(t, 3)
	engines[0].ScheduleAt(5, retireEngineAction{Engine: 2})
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
		t.Fatalf("%d records of its retirement, want 1", retired)
	}
	if _, ok := c.Find(1); !ok {
		t.Fatal("the engine that asked was retired too")
	}
}

func TestEnginesAskedToBeRetiredFromManyGoroutinesWhileOthersRunAreAllRetired(t *testing.T) {
	const n = 12
	c, _, log := toysThatCanRetireOneAnother(t, n)
	var wg sync.WaitGroup
	for id := sim.StateID(2); id <= n; id += 2 {
		wg.Go(func() {
			time.Sleep(time.Duration(id) * time.Millisecond)
			c.RequestEngineRetirement(id)
			c.RequestEngineRetirement(id) // a second request changes nothing
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
				t.Fatalf("the log of engine %d does not end with its retirement", id)
			}
		}
	}
}

func lastBody(log *eventlog.MemLog, id sim.StateID) eventlog.Body {
	records := log.ByEngine()[id]
	return records[len(records)-1].Body
}
