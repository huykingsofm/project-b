package cluster_test

import (
	"slices"
	"testing"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/cluster"
	"github.com/huykingsofm/project-b/platform/runtime/cluster/stepping"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

// createEngineAction asks for an engine to be made, and sends the next number's engine a message in
// the same tick when Send is set.
type createEngineAction struct {
	Total int64
	Send  sim.StateID
}

func (createEngineAction) ID() sim.ActionID { return 60012 }
func (createEngineAction) Priority() int16  { return 0 }
func (createEngineAction) Actor() any       { return nil }
func (a createEngineAction) Apply(
	_ *sim.Context, _ *testkit.ToyState, r *sim.ActionRegistry[*testkit.ToyState],
	_ *sim.Plan[*testkit.ToyState],
) {
	r.RequestCreateEngine(
		[]byte("toy"), &testkit.ToyState{Total: a.Total}, testkit.ToyAction{N: 10},
	)
	if a.Send != 0 {
		r.Send(a.Send, onfail.Retry{}, testkit.ToyAction{N: 3})
	}
}
func (a createEngineAction) Clone() sim.Action[*testkit.ToyState] { return a }

// toysThatCanMakeOneAnother is a cluster of the toy engines 1 to n that has the registry of the toy
// engines, and the log that it writes.
func toysThatCanMakeOneAnother(
	t testing.TB, n int,
) (*cluster.Cluster, []*engine.Engine[*testkit.ToyState], *eventlog.MemLog) {
	t.Helper()
	return toysThatCanRemoveOneAnother(t, n)
}

func steps(t *testing.T, c *cluster.Cluster, n int) {
	t.Helper()
	for range n {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
}

func dependencyOf(r eventlog.Record) (sim.StateDependency, bool) { return r.Dependency() }

func TestAnEngineThatARequestMadeRunsOnlyAfterItsCreatorTookTheResponse(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(3, createEngineAction{Total: 50})
	steps(t, c, 3) // the tick 3 of the engine 1 asks: the request is published at its end
	if _, ok := c.Find(3); ok {
		t.Fatal("the engine 3 is in the cluster before the cluster served the request")
	}
	steps(t, c, 1) // the cluster makes it and answers; the creator takes the answer at its tick 4
	if _, ok := c.Find(3); ok {
		t.Fatal("the engine 3 is in the cluster before its creator acknowledged the response")
	}
	if reason, ok := c.Env.Router.Deliver(testkit.ToyMessage(3, 1)); ok ||
		reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("a message to the engine before it joined the router: %v, %v", reason, ok)
	}
	records := log.ByEngine()[3]
	init, isInit := records[0].Body.(eventlog.Init)
	if !isInit || records[0].Tick != 1 || init.Creator != 1 || init.CreatorTick != 3 ||
		init.DirectiveID != 1 || len(init.Snapshot) == 0 {
		t.Fatalf("the log of the engine 3 is %+v, want Init first, at the tick 1, for the "+
			"request 1 that engine 1 made at its tick 3", records)
	}
	for _, r := range records {
		if r.Tick != 1 {
			t.Fatalf("the engine 3 wrote %+v after its tick 1 before it was taken in", r)
		}
	}
	if dep, ok := dependencyOf(records[0]); !ok ||
		dep != (sim.StateDependency{Source: 1, Tick: 3}) {
		t.Fatalf("Init depends on %+v, %v, want the tick 3 of the engine 1", dep, ok)
	}
	steps(t, c, 1) // the cluster takes the engine in, and it runs its tick 2
	found, ok := c.Find(3)
	if !ok {
		t.Fatal("the engine 3 is not in the cluster after its creator acknowledged")
	}
	made := found.(*engine.Engine[*testkit.ToyState])
	if made.Tick() != 2 {
		t.Fatalf("the engine ran to the tick %d, want 2", made.Tick())
	}
	var startedRecord eventlog.Record
	for _, r := range log.ByEngine()[3] {
		if _, isStarted := r.Body.(eventlog.Started); isStarted {
			startedRecord = r
		}
	}
	started, isStarted := startedRecord.Body.(eventlog.Started)
	if !isStarted || startedRecord.Tick != 2 || started.Creator != 1 || started.ResponseTick != 4 {
		t.Fatalf("Started is %+v, want it at the tick 2 for the response that the engine 1 took "+
			"at its tick 4", startedRecord)
	}
	if dep, ok := dependencyOf(startedRecord); !ok ||
		dep != (sim.StateDependency{Source: 1, Tick: 4}) {
		t.Fatalf("Started depends on %+v, %v, want the tick 4 of the engine 1", dep, ok)
	}
	// the actions it starts with run at the tick 2: the 50 it was made with and the 10
	if got := made.State().Total; got != 60 {
		t.Fatalf("the new engine has %d, want 60", got)
	}
	if got := engines[0].State().Total; got != 300 {
		t.Fatalf("the creator has %d, want 300: the answer for the engine 3", got)
	}
	for _, r := range log.ByEngine()[1] {
		d, isDirective := r.Body.(eventlog.Directive)
		if !isDirective {
			continue
		}
		dep, ok := dependencyOf(r)
		if r.Tick != 4 || d.Response.Engine != 3 || !ok ||
			dep != (sim.StateDependency{Source: 3, Tick: 1}) {
			t.Fatalf("the answer %+v at the tick %d depends on %+v, %v: want the tick 1 of "+
				"engine 3", d.Response, r.Tick, dep, ok)
		}
	}
}

func TestAnEngineThatRunsOnAnEarlierRequestOfItsCreatorIsNotMadeTwice(t *testing.T) {
	c, engines, _ := toysThatCanMakeOneAnother(t, 1)
	engines[0].ScheduleAt(1, createEngineAction{})
	engines[0].ScheduleAt(2, createEngineAction{})
	steps(t, c, 8)
	if got := len(c.AllOfType[*testkit.ToyState]()); got != 3 {
		t.Fatalf("%d toy engines, want 3: the creator and the two it asked for", got)
	}
	for _, id := range []sim.StateID{2, 3} {
		if _, ok := c.Find(id); !ok {
			t.Fatalf("the engine %v is not in the cluster", id)
		}
	}
}

func TestAnEngineWhoseCreatorIsGoneBeforeItTookTheResponseIsEndedAndNeverRuns(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(1, createEngineAction{})
	steps(t, c, 1)                    // the engine 1 asks
	c.RunPeriodicTasks()              // the cluster makes the engine 3 and answers
	if !stepping.RetireEngine(c, 1) { // the creator is gone before it took the answer
		t.Fatal("the engine 1 is not in the cluster")
	}
	c.RunPeriodicTasks()
	steps(t, c, 2)
	if _, ok := c.Find(3); ok {
		t.Fatal("the engine of a creator that is gone runs")
	}
	var kinds []eventlog.Kind
	for _, r := range log.ByEngine()[3] {
		kinds = append(kinds, r.Kind())
	}
	last := len(kinds) - 1
	if len(kinds) < 2 || kinds[0] != eventlog.KindInit || kinds[last] != eventlog.KindRetired {
		t.Fatalf("the log of the engine 3 holds the kinds %v, want Init first and Retired last",
			kinds)
	}
	for _, kind := range kinds {
		if kind == eventlog.KindStarted {
			t.Fatal("an engine that never ran wrote Started")
		}
	}
}

func TestAnEngineWhoseCreatorTookTheResponseAndThenRetiredRunsAndIsNotEnded(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(1, createEngineAction{})
	steps(t, c, 1) // the engine 1 asks
	// the cluster makes the engine 3 and answers; the engine 1 takes the answer at its tick 2 and
	// acknowledges at its end
	steps(t, c, 1)
	if !stepping.RetireEngine(c, 1) { // it retires before the cluster took the acknowledgement
		t.Fatal("the engine 1 is not in the cluster")
	}
	steps(t, c, 2)
	found, ok := c.Find(3)
	if !ok {
		t.Fatal("the engine that its creator took the response for was ended when its creator left")
	}
	if made := found.(*engine.Engine[*testkit.ToyState]); made.Tick() != 3 {
		t.Fatalf("the engine 3 ran to the tick %d, want 3", made.Tick())
	}
	var kinds []eventlog.Kind
	for _, r := range log.ByEngine()[3] {
		kinds = append(kinds, r.Kind())
		if started, ok := r.Body.(eventlog.Started); ok &&
			(r.Tick != 2 || started.Creator != 1 || started.ResponseTick != 2) {
			t.Fatalf("Started is %+v, want it at the tick 2 for the response that the engine 1 "+
				"took at its tick 2", r)
		}
	}
	if !slices.Contains(kinds, eventlog.KindStarted) ||
		slices.Contains(kinds, eventlog.KindRetired) {
		t.Fatalf("the log of the engine 3 holds the kinds %v, want Started and no Retired", kinds)
	}
}

func TestACreatorIsToldWhenTheEngineCannotBeMade(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 1)
	// the state is of a type that no registry knows
	engines[0].ScheduleAt(1, askForAnEngineOfAnotherTypeAction{})
	steps(t, c, 4)
	if got := engines[0].State().Total; got != -1 {
		t.Fatalf("the creator has %d, want -1: the answer says it was not made", got)
	}
	for _, r := range log.ByEngine()[1] {
		if d, ok := r.Body.(eventlog.Directive); ok {
			if d.Response.Failure == "" || d.Response.Engine != 0 {
				t.Fatalf("the answer is %+v, want a failure", d.Response)
			}
			if _, depends := dependencyOf(r); depends {
				t.Fatal("an answer that failed depends on an engine")
			}
		}
	}
}

// askForAnEngineOfAnotherTypeAction asks for an engine of a state that is of a type nobody
// registered.
type askForAnEngineOfAnotherTypeAction struct{}

func (askForAnEngineOfAnotherTypeAction) ID() sim.ActionID { return 60013 }
func (askForAnEngineOfAnotherTypeAction) Priority() int16  { return 0 }
func (askForAnEngineOfAnotherTypeAction) Actor() any       { return nil }
func (askForAnEngineOfAnotherTypeAction) Apply(
	_ *sim.Context, _ *testkit.ToyState, r *sim.ActionRegistry[*testkit.ToyState],
	_ *sim.Plan[*testkit.ToyState],
) {
	r.RequestCreateEngineOf([]byte("toy"), &unregisteredToy{}, nil)
}
func (a askForAnEngineOfAnotherTypeAction) Clone() sim.Action[*testkit.ToyState] { return a }

type unregisteredToy struct{ testkit.ToyState }

func (*unregisteredToy) Type() sim.StateType { return testkit.ToyType + 5 }
func (s *unregisteredToy) Clone() sim.State  { c := *s; return &c }

func TestTheClusterGivesTheNumbersOfATypeInOrderAndNeverGivesOneAgain(t *testing.T) {
	c, engines, _ := toysThatCanMakeOneAnother(t, 1)
	engines[0].ScheduleAt(1, createEngineAction{})
	engines[0].ScheduleAt(2, createEngineAction{})
	steps(t, c, 8)
	for _, id := range []sim.StateID{1, 2, 3} {
		if _, ok := c.Find(id); !ok {
			t.Fatalf("the engine %v is not in the cluster", id)
		}
	}
	next, err := c.Orchestrator().NextStateID(testkit.ToyType)
	if err != nil || next != 4 {
		t.Fatalf("the next number is %v, %v, want 4", next, err)
	}
}

func TestAnEngineThatIsMadeWhileTheClusterRunsRunsOnItsOwnGoroutine(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(5, createEngineAction{Total: 50})
	runUntil(t, c, func() bool {
		for _, r := range log.ByEngine()[3] {
			if r.Tick > 3 { // it ran ticks after Started, on its own goroutine
				return true
			}
		}
		return false
	})
	kinds := map[eventlog.Kind]int{}
	for _, r := range log.ByEngine()[3] {
		kinds[r.Kind()]++
	}
	if kinds[eventlog.KindInit] != 1 || kinds[eventlog.KindStarted] != 1 {
		t.Fatalf("the log of the engine 3 holds %v, want one Init and one Started", kinds)
	}
}

func TestARequestThatWaitsInACheckpointIsPublishedAgainByTheEngineThatIsMadeFromIt(t *testing.T) {
	env := testkit.NewToyEnv(t)
	a := engine.NewEngine(&testkit.ToyState{}, 1, env)
	a.ScheduleAt(1, createEngineAction{Total: 7})
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	if got := env.DirectiveRouter.TakeRequests(); len(got) != 1 {
		t.Fatalf("%d requests published, want 1", len(got))
	}
	// the cluster that held the request is gone: nothing answered it
	data, err := a.EncodeCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	again := testkit.NewToyEnv(t)
	restored, err := again.EngineRegistry.Restore(again, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.(*engine.Engine[*testkit.ToyState]).Step(); err != nil {
		t.Fatal(err)
	}
	got := again.DirectiveRouter.TakeRequests()
	if len(got) != 1 || got[0].Request.ID != 1 || got[0].Request.Tick != 1 ||
		got[0].Request.State.(*testkit.ToyState).Total != 7 {
		t.Fatalf("the engine made again published %+v, want the request 1 that it made at its "+
			"tick 1", got)
	}
	if err := restored.(*engine.Engine[*testkit.ToyState]).Step(); err != nil {
		t.Fatal(err)
	}
	if got := again.DirectiveRouter.TakeRequests(); len(got) != 0 {
		t.Fatalf("%d requests published again at its second tick, want none", len(got))
	}
}
