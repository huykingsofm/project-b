package engine_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/engine/stepping"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/logging"
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
	_ *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	r.RequestCreateEngine([]byte("toy"), &toyState{Total: a.Total}, toyAction{N: 10})
	if a.Send != 0 {
		r.Send(a.Send, onfail.Retry{}, toyAction{N: 3})
	}
}
func (a createEngineAction) Clone() sim.Action[*toyState] { return a }

// toysThatCanMakeOneAnother is a cluster of the toy engines 1 to n that has the registry of the toy
// engines, and has given the numbers up to n.
func toysThatCanMakeOneAnother(
	t testing.TB, n int,
) (*engine.Cluster, []*engine.Engine[*toyState], *eventlog.MemLog) {
	t.Helper()
	c, engines, log := toysThatCanRemoveOneAnother(t, n)
	c.RaiseStateIDs(map[sim.StateType]uint64{toyType: uint64(n)})
	return c, engines, log
}

func steps(t *testing.T, c *engine.Cluster, n int) {
	t.Helper()
	for range n {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
}

func aRequest(id uint64) sim.DirectiveRequest {
	return sim.DirectiveRequest{
		ID: id, Kind: sim.DirectiveCreateEngine, Tag: []byte("toy"), State: &toyState{},
	}
}

func TestAnEngineThatAnActionAskedForGetsTheNextIDRunsFromTheNextStepAndBeginsItsLogWithItsStart(
	t *testing.T,
) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(3, createEngineAction{Total: 50})
	steps(t, c, 3)
	found, ok := c.Find(3) // the toy engines 1 and 2 are there: the next number is 3
	if !ok {
		t.Fatal("engine 3 is not in the cluster")
	}
	made := found.(*engine.Engine[*toyState])
	if made.Tick() != 0 {
		t.Fatalf("engine 3 is at the tick %d, want it made and not run", made.Tick())
	}
	steps(t, c, 1)
	if made.Tick() != 1 {
		t.Fatalf("the engine ran to the tick %d in the step after, want 1", made.Tick())
	}
	records := log.ByEngine()[3]
	started, isStarted := records[0].Body.(eventlog.Started)
	if !isStarted || records[0].Tick != 1 || started.Creator != 1 || started.CreatorTick != 3 ||
		started.DirectiveID != 1 || len(started.Snapshot) == 0 {
		t.Fatalf("the first record of its log is %+v, want the start by engine 1 at its tick 3, "+
			"for the request 1", records[0])
	}
	for _, r := range records[1:] {
		if _, again := r.Body.(eventlog.Started); again {
			t.Fatal("the start is written more than once")
		}
	}
	// the engine starts as the request made it: its state, and the action it starts with
	steps(t, c, 1)
	if got := made.State().Total; got != 60 {
		t.Fatalf("the new engine has %d, want 60: the 50 it was made with and the 10 it starts"+
			" with", got)
	}
	// the creator took the answer at the start of the tick after the one that asked
	if got := engines[0].State().Total; got != 300 {
		t.Fatalf("the creator has %d, want 300: the answer for the engine 3", got)
	}
	answers := 0
	for _, r := range log.ByEngine()[1] {
		if d, ok := r.Body.(eventlog.Directive); ok {
			answers++
			if d.Response.ID != 1 || d.Response.Engine != 3 || string(d.Response.Tag) != "toy" {
				t.Fatalf("the answer in its log is %+v", d.Response)
			}
		}
	}
	if answers != 1 {
		t.Fatalf("%d answers in the log of the creator, want 1", answers)
	}
}

func TestAMessageThatTheTickOfTheRequestSendsToTheNewEngineIsNotRefusedAsNotStarted(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(1, createEngineAction{Send: 3})
	steps(t, c, 3)
	for _, r := range log.ByEngine()[1] {
		if _, returned := r.Body.(eventlog.Returned); returned {
			t.Fatalf("a message came back to the creator: %+v", r)
		}
	}
	made, _ := c.Find(3)
	// the message, and the action it starts with
	if made.(*engine.Engine[*toyState]).State().Total != 13 {
		t.Fatal("the new engine did not take the message of the tick that made it")
	}
}

func TestACreatorIsToldWhenTheEngineCannotBeMade(t *testing.T) {
	c, engines, _ := toysThatCanMakeOneAnother(t, 2)
	creator := engines[0]
	other := aRequest(1)
	other.State = &otherToy{} // a state of a type that the registry does not know
	c.ServeDirectives(1, 1, []sim.DirectiveRequest{other})
	c.Env.EngineRegistry = nil
	c.ServeDirectives(1, 1, []sim.DirectiveRequest{aRequest(2)}) // there is no registry
	c.ServeDirectives(1, 1, []sim.DirectiveRequest{{ID: 3, Kind: 99}})
	if got := len(c.Engines()); got != 3 {
		t.Fatalf("the cluster has %d engines, want the 3 it had", got)
	}
	steps(t, c, 1)
	// the creator is not waiting for these answers, which it did not ask for: they are dropped
	if got := creator.State().Total; got != 0 {
		t.Fatalf("the creator has %d, want 0: it took answers to requests it did not make", got)
	}
}

func TestAnAnswerIsTakenOnlyForARequestThatIsWaiting(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 1)
	c.ServeDirectives(1, 1, []sim.DirectiveRequest{aRequest(1)}) // 1 never asked
	steps(t, c, 2)
	if got := engines[0].State().Total; got != 0 {
		t.Fatalf("the engine has %d, want 0: nothing was waiting for the answer", got)
	}
	for _, r := range log.ByEngine()[1] {
		if _, ok := r.Body.(eventlog.Directive); ok {
			t.Fatal("an answer that nothing waited for was written to the log")
		}
	}
}

func TestARequestThatIsHandedOverAgainIsServedOnceAndAnsweredOnce(t *testing.T) {
	c, engines, _ := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(1, createEngineAction{})
	steps(t, c, 2)
	if got := engines[0].State().Total; got != 300 {
		t.Fatalf("the creator has %d, want 300", got)
	}
	// the cluster is given the request again, as an engine that does not hold the answer does
	again := sim.DirectiveRequest{
		ID: 1, Kind: sim.DirectiveCreateEngine, Tag: []byte("toy"), State: &toyState{},
	}
	c.ServeDirectives(1, 1, []sim.DirectiveRequest{again, again})
	steps(t, c, 2)
	if got := len(c.Engines()); got != 4 { // the configuration engine, the two and the one made
		t.Fatalf("the cluster has %d engines, want 4: the request was served again", got)
	}
	if got := engines[0].State().Total; got != 300 {
		t.Fatalf("the creator has %d, want 300: it took the answer twice", got)
	}
}

func TestARequestWithNoAnswerIsHandedOverAgainAndItsAnswerComesThen(t *testing.T) {
	c, engines, _ := toysThatCanMakeOneAnother(t, 2)
	setConfig(t, c, map[string]string{"directive.resend_every_ticks": "3"})
	creator := engines[0]
	for creator.HandleDirectiveResponse(sim.DirectiveResponse{ID: 99}) == nil {
		// until the queue of answers is full: the answer cannot be taken now
	}
	creator.ScheduleAt(1, createEngineAction{})
	steps(t, c, 1)
	if _, ok := c.Find(3); !ok {
		t.Fatal("the engine was not made")
	}
	// the queue is emptied at the start of a tick; the request goes again at the tick 3 or 6
	steps(t, c, 7)
	if got := creator.State().Total; got != 300 {
		t.Fatalf("the creator has %d, want 300: the answer came with the second hand-over", got)
	}
	if got := len(c.Engines()); got != 4 {
		t.Fatalf("the cluster has %d engines, want 4: one engine for the request", got)
	}
}

func TestAnAnswerForAnEngineThatIsGoneIsLostAndTheLogHoldsItAsDataToFix(t *testing.T) {
	logged := &loggedEntries{}
	stopLogging, err := logging.Init(logged, logging.WithFlushEvery(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := toysThatCanMakeOneAnother(t, 2)
	c.ServeDirectives(77, 1, []sim.DirectiveRequest{aRequest(1)}) // no engine 77
	stopLogging()                                                 // gives the consumer what is left
	logged.mu.Lock()
	defer logged.mu.Unlock()
	fixes := 0
	for _, entry := range logged.entries {
		if strings.HasPrefix(entry.Message, "datafix:") {
			fixes++
		}
	}
	if fixes != 1 {
		t.Fatalf("%d lines of data to fix, want 1", fixes)
	}
}

// loggedEntries collects the entries of the log, for a test that reads what was logged.
type loggedEntries struct {
	mu      sync.Mutex
	entries []logging.Entry
}

func (l *loggedEntries) Consume(entries []logging.Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entries...)
	return nil
}

func TestTheClusterGivesTheNumbersOfATypeInOrderAndNeverGivesOneAgain(t *testing.T) {
	c, _, _ := toysThatCanMakeOneAnother(t, 2)
	if got := c.NewStateID(toyType); got != sim.NewStateID(toyType, 3) {
		t.Fatalf("the next ID is %v, want the number 3", got)
	}
	c.RaiseStateIDs(map[sim.StateType]uint64{toyType: 10, toyType + 1: 4})
	c.RaiseStateIDs(map[sim.StateType]uint64{toyType: 5}) // never goes down
	if got := c.NewStateID(toyType); got != sim.NewStateID(toyType, 11) {
		t.Fatalf("the next ID is %v, want the number 11", got)
	}
	if got := c.NewStateID(toyType + 1); got != sim.NewStateID(toyType+1, 5) {
		t.Fatalf("the next ID of the other type is %v, want the number 5", got)
	}
}

func TestAnEngineAddedToTheClusterGetsAnIDAndAStartThatHoldsNoCreator(t *testing.T) {
	c, _, log := toysThatCanMakeOneAnother(t, 0)
	id, err := c.CreateEngine(&toyState{Total: 7}, toyAction{N: 1})
	if err != nil || id != sim.NewStateID(toyType, 1) {
		t.Fatalf("the engine has the ID %v, error %v, want the number 1", id, err)
	}
	steps(t, c, 1)
	started, ok := log.ByEngine()[id][0].Body.(eventlog.Started)
	if !ok || started.Creator != 0 || started.CreatorTick != 0 || started.DirectiveID != 0 {
		t.Fatalf("the first record of the engine is %+v", log.ByEngine()[id][0])
	}
	if _, dependent := (eventlog.Record{Body: started}).Dependency(); dependent {
		t.Fatal("an engine that no engine asked for depends on another")
	}
	e, _ := c.Find(id)
	if got := e.(*engine.Engine[*toyState]).State().Total; got != 8 {
		t.Fatalf("the engine has %d, want 8: its state and the action it starts with", got)
	}
}

func TestAnEngineThatIsMadeWhileTheClusterRunsStartsOnItsOwnGoroutine(t *testing.T) {
	c, engines, log := toysThatCanMakeOneAnother(t, 2)
	engines[0].ScheduleAt(5, createEngineAction{})
	// the engine writes the record of its start at its first tick, on its goroutine
	runUntil(t, c, func() bool { return len(log.ByEngine()[3]) > 0 })
	if _, ok := log.ByEngine()[3][0].Body.(eventlog.Started); !ok {
		t.Fatalf("the first record of the engine is %+v", log.ByEngine()[3][0])
	}
}

func TestEnginesMadeAndRemovedFromManyGoroutinesWhileOthersRunLeaveACoherentCluster(t *testing.T) {
	c, _, log := toysThatCanMakeOneAnother(t, 4)
	const made = 20
	var wg sync.WaitGroup
	for i := range made {
		wg.Go(func() {
			c.ServeDirectives(1, 1, []sim.DirectiveRequest{aRequest(uint64(i + 1))})
		})
	}
	runUntil(t, c, func() bool { return len(c.Engines()) == 1+4+made })
	wg.Wait()
	// the numbers 5 to 24 were given, each once; the even ones are removed
	var removing sync.WaitGroup
	for n := uint64(5); n < 5+made; n += 2 {
		removing.Go(func() { c.RemoveEngine(sim.NewStateID(toyType, n)) })
	}
	removing.Wait()
	runUntil(t, c, func() bool {
		for n := uint64(5); n < 5+made; n++ {
			_, ok := c.Find(sim.NewStateID(toyType, n))
			if ok != (n%2 == 0) {
				return false
			}
		}
		return true
	})
	for n := uint64(5); n < 5+made; n += 2 {
		if _, ok := lastBody(log, sim.NewStateID(toyType, n)).(eventlog.Retired); !ok {
			t.Fatalf("the log of engine %d does not end with its removal", n)
		}
	}
}

func TestARequestThatWaitsInACheckpointIsHandedOverByTheEngineThatIsMadeFromIt(t *testing.T) {
	// an engine whose cluster does not answer: the request waits in its data
	env := newToyEnv(t)
	a := engine.NewEngine(&toyState{}, 1, env)
	a.ScheduleAt(1, createEngineAction{})
	if err := a.Step(); err != nil {
		t.Fatal(err)
	}
	data, err := a.EncodeCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	// the cluster that is made again serves the request when the engine made from the checkpoint
	// hands it over at its first tick
	fresh := newToyEnv(t)
	c := newCluster(t, fresh)
	restored, err := c.AddEngineByCheckpoint(data)
	if err != nil {
		t.Fatal(err)
	}
	c.RaiseStateIDs(map[sim.StateType]uint64{toyType: 1})
	steps(t, c, 2)
	if _, ok := c.Find(2); !ok {
		t.Fatal("the request that waited in the checkpoint was not served")
	}
	if got := restored.(*engine.Engine[*toyState]).State().Total; got != 200 {
		t.Fatalf("the engine has %d, want 200: the answer for the engine 2", got)
	}
}
