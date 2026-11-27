package engine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/huykingsofm/project-b/platform/config"
	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/engine/stepping"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
)

// createEngineAction asks the coordinator for an engine of a toy state with the total given, which
// starts by adding 10 to it.
type createEngineAction struct{ Total int64 }

func (createEngineAction) ID() sim.ActionID { return 60012 }
func (createEngineAction) Priority() int16  { return 0 }
func (createEngineAction) Actor() any       { return nil }
func (a createEngineAction) Apply(
	_ *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	r.RequestCreateEngine(onfail.Drop{}, &toyState{Total: a.Total}, toyAction{N: 10})
}
func (a createEngineAction) Clone() sim.Action[*toyState] { return a }

// createUnknownEngineAction asks the coordinator for an engine of a state that no registry knows.
type createUnknownEngineAction struct{}

func (createUnknownEngineAction) ID() sim.ActionID { return 60013 }
func (createUnknownEngineAction) Priority() int16  { return 0 }
func (createUnknownEngineAction) Actor() any       { return nil }
func (createUnknownEngineAction) Apply(
	_ *sim.Context, _ *toyState, r *sim.Registry[*toyState], _ *sim.Plan[*toyState],
) {
	r.RequestCreateEngine[*otherToy](onfail.Drop{}, &otherToy{})
}
func (a createUnknownEngineAction) Clone() sim.Action[*toyState] { return a }

// toysWithALog is a cluster of the toy engines 1 to n, and the log that it writes.
func toysWithALog(
	t testing.TB, n int,
) (*engine.Cluster, []*engine.Engine[*toyState], *eventlog.MemLog) {
	t.Helper()
	env := newToyEnv(t)
	log := &eventlog.MemLog{}
	env.EventLog = log
	c := newCluster(t, env)
	return c, addToys(t, c, n), log
}

func steps(t *testing.T, c *engine.Cluster, n int) {
	t.Helper()
	for range n {
		if err := stepping.Step(c); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnEngineThatAnActionAskedForGetsTheNextIDRunsFromTheNextStepAndBeginsItsLogWithItsStart(
	t *testing.T,
) {
	c, engines, log := toysWithALog(t, 2)
	engines[0].ScheduleAt(1, createEngineAction{Total: 50})
	steps(t, c, 1) // the engine asks: the message is in the queue of the coordinator
	if _, found := c.Find(3); found {
		t.Fatal("the engine was made before the coordinator took the request")
	}
	steps(t, c, 1) // the coordinator takes it and makes the engine, which does not run in this step
	made, found := c.Find(3)
	if !found {
		t.Fatal("the engine 3 is not in the cluster after the coordinator took the request")
	}
	three := made.(*engine.Engine[*toyState])
	if three.Tick() != 0 || three.State().Total != 50 {
		t.Fatalf("the engine is at the tick %d with %d, want it made and not run, with 50",
			three.Tick(), three.State().Total)
	}
	madeAt := c.Coordinator().Tick()
	steps(t, c, 1)
	if three.Tick() != 1 || three.State().Total != 60 {
		t.Fatalf("the engine ran to the tick %d with %d, want 1 with 60: the 50 it was made with "+
			"and the 10 it starts with", three.Tick(), three.State().Total)
	}
	steps(t, c, 1)
	if three.State().Total != 60 {
		t.Fatalf("the engine has %d, want 60: what it starts with runs once", three.State().Total)
	}
	records := log.ByEngine()[3]
	started, isStarted := records[0].Body.(eventlog.Started)
	if !isStarted || records[0].Tick != 1 || started.Creator != sim.CoordinatorStateID ||
		started.CreatorTick != madeAt {
		t.Fatalf("the log of the engine begins with %+v, want Started by the coordinator at its "+
			"tick %d", records[0], madeAt)
	}
	dep, _ := records[0].Dependency()
	if dep.Source != sim.CoordinatorStateID || dep.Tick != madeAt {
		t.Fatalf("the engine depends on %+v, want the tick %d of the coordinator", dep, madeAt)
	}
}

func TestAnEngineThatCannotBeMadeIsNotMadeAndTheNextOneIsMade(t *testing.T) {
	c, engines, _ := toysWithALog(t, 1)
	engines[0].ScheduleAt(1, createUnknownEngineAction{})
	engines[0].ScheduleAt(2, createEngineAction{})
	steps(t, c, 4)
	if _, found := c.Find(2); !found {
		t.Fatal("the engine that the registry knows was not made after one that it does not")
	}
	if got := len(c.Engines()); got != 4 { // the configuration engine, the coordinator, 1 and 2
		t.Fatalf("the cluster has %d engines, want 4", got)
	}
}

func TestACreationThatCannotBeMadeIsAnErrorOfCreateEngine(t *testing.T) {
	c, _, _ := toysWithALog(t, 1)
	if _, err := c.CreateEngine(&otherToy{}); err == nil {
		t.Fatal("an engine of a state that no registry knows was made")
	}
	if id, err := c.CreateEngine(&toyState{}); err != nil || id != 2 {
		t.Fatalf("the next engine is %v (%v), want 2", id, err)
	}
}

func TestARequestThatIsDeliveredTwiceMakesOneEngine(t *testing.T) {
	c, _, _ := toysWithALog(t, 2)
	ask, err := sim.NewCreateEngineAction(&toyState{Total: 5}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	request := sim.NewActionMessage[*sim.CoordinatorState](
		sim.CoordinatorStateID, onfail.Drop{}, ask,
	).Stamp(sim.Header{From: 1, Seq: 77, Tick: 1})
	for range 2 { // the sender did not get an ack and sent it again
		if _, ok := c.Env.Router.Deliver(request); !ok {
			t.Fatal("the request was refused")
		}
	}
	steps(t, c, 2)
	if _, found := c.Find(3); !found {
		t.Fatal("the request made no engine")
	}
	if _, found := c.Find(4); found {
		t.Fatal("the same request made two engines")
	}
}

func TestTheNumbersGoOnFromTheCheckpointOfTheCoordinator(t *testing.T) {
	c, _, _ := toysWithALog(t, 2)
	data, err := c.Coordinator().EncodeCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	again, err := engine.NewCluster(
		newToyEnv(t), config.NewStore(), engine.Checkpoints{Coordinator: data},
	)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := again.CreateEngine(&toyState{}); err != nil || id != 3 {
		t.Fatalf("the engine made after the restore is %v (%v), want 3: 1 and 2 are given", id, err)
	}
}

func TestACoordinatorOfAReplayMakesNoEngine(t *testing.T) {
	c, err := engine.NewReplayCluster(newToyEnv(t), engine.Checkpoints{})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := sim.NewCreateEngineCommand(&toyState{}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Coordinator().Send(cmd) {
		t.Fatal("the command was refused")
	}
	steps(t, c, 2)
	if got := len(c.Engines()); got != 2 { // the configuration engine and the coordinator
		t.Fatalf("the replay has %d engines, want 2: its engines are made from its logs", got)
	}
}

func TestEnginesMadeWhileTheClusterRunsGetDifferentNumbersAndStartOnTheirOwnGoroutines(
	t *testing.T,
) {
	c, _, _ := toysWithALog(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { c.Run(ctx, time.Millisecond); close(finished) }()
	const askers, each = 6, 5
	ids := make(chan sim.StateID, askers*each)
	var wg sync.WaitGroup
	for range askers {
		wg.Go(func() {
			for range each {
				id, err := c.CreateEngine(&toyState{})
				if err != nil {
					t.Error(err)
					return
				}
				ids <- id
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := map[sim.StateID]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("the number %v was given twice", id)
		}
		seen[id] = true
	}
	deadline := time.After(2 * time.Second)
	published := c.Env.StableStore.(*engine.StableStateStore)
	for id := range seen {
		for {
			// what an engine published is read from any goroutine; its tick is not
			if copy := published.Copy(0, 0, id); copy != nil && copy.Tick() > 0 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("the engine %v did not run on its own goroutine", id)
			case <-time.After(time.Millisecond):
			}
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if len(seen) != askers*each {
		t.Fatalf("%d numbers were given, want %d", len(seen), askers*each)
	}
}
