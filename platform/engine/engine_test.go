package engine_test

import (
	"reflect"
	"testing"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/engine/stepping"
	"github.com/huykingsofm/project-b/platform/eventlog"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

func newToyEnv(t testing.TB) *engine.Environment {
	t.Helper()
	codecs, engines, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	return &engine.Environment{
		Router: engine.NewRouter(), EventLog: &eventlog.MemLog{},
		StableStore: engine.NewStableStateStore(), Codecs: codecs, EngineRegistry: engines, Seed: 1,
		QueueSize: 16,
		Emitter:   engine.NopEmitter{},
		AckBoard:  engine.NewAckBoard(),
	}
}

// toyCluster is a cluster with n toy engines, which the cluster gives the numbers 1 to n.
func toyCluster(t testing.TB, n int) (*engine.Cluster, []*engine.Engine[*toyState]) {
	t.Helper()
	c := newCluster(t, newToyEnv(t))
	return c, addToys(t, c, n)
}

func TestAStepRunsTheCommandsPublishesTheStateAndRecordsTheCommand(t *testing.T) {
	c, engines := toyCluster(t, 1)
	a := engines[0]
	if !a.Send(toyCommand{N: 5}) {
		t.Fatal("the command was refused")
	}
	stepping.Step(c)
	if a.State().Total != 5 || a.Tick() != 1 {
		t.Fatalf("total %d at tick %d, want 5 at 1", a.State().Total, a.Tick())
	}
	published, err := c.Env.StableStore.View(2, eventlog.NopLog{}).Read(1)
	if err != nil || published.(*toyState).Total != 5 || published.(*toyState) == a.State() {
		t.Fatalf("the published state is %+v, %v: want a copy with the total", published, err)
	}
	mem := c.Env.EventLog.(*eventlog.MemLog)
	commands := 0
	for _, r := range mem.Records {
		if r.Kind() == eventlog.KindCommand && r.Engine == 1 && r.Tick == 1 {
			commands++
		}
	}
	if commands != 1 {
		t.Fatalf("%d commands recorded, want 1", commands)
	}
}

func TestAMessageToAnotherEngineIsTakenAtItsNextTickAndACopyIsNeverTakenTwice(t *testing.T) {
	c, engines := toyCluster(t, 2)
	a, b := engines[0], engines[1]
	a.ScheduleAt(1, forwardAction{To: 2, N: 7})
	stepping.Step(c) // a sends: b runs after a in this step, so it takes the message at once
	if b.State().Total != 7 {
		t.Fatalf("b has %d, want 7", b.State().Total)
	}
	dup := sim.NewActionMessage(2, onfail.Drop{}, toyAction{N: 100}).
		Stamp(sim.Header{From: 1, Seq: 1}) // the first message of a had the number 1
	c.Env.Router.Deliver(dup)
	stepping.Step(c)
	if b.State().Total != 7 {
		t.Fatalf("a second copy of a message was taken: %d", b.State().Total)
	}
}

func TestAMessageCarriesTheTickOfItsSenderAndTheReceiverDependsOnIt(t *testing.T) {
	c, engines := toyCluster(t, 2)
	a := engines[0]
	a.ScheduleAt(3, forwardAction{To: 2, N: 7})
	stepping.Step(c)
	stepping.Step(c)
	stepping.Step(c) // a sends at its tick 3: b takes the message in the same step
	mem := c.Env.EventLog.(*eventlog.MemLog)
	var deps []sim.StateDependency
	for _, r := range mem.Records {
		// every engine reads the configuration at every tick: that is not what is asked here
		if dep, ok := r.Dependency(); ok && r.Engine == 2 && dep.Source != sim.ConfigStateID &&
			dep.Source != sim.CoordinatorStateID {
			deps = append(deps, dep)
		}
	}
	if want := []sim.StateDependency{{Source: 1, Tick: 3}}; !reflect.DeepEqual(deps, want) {
		t.Fatalf("the dependencies of b are %v, want %v", deps, want)
	}
}

func TestAnEngineReadsWhatAnotherPublishedByItsID(t *testing.T) {
	c, engines := toyCluster(t, 2)
	engines[0].Send(toyCommand{N: 9})
	stepping.Step(c)
	engines[1].ScheduleAt(2, readAction{From: 1})
	stepping.Step(c)
	if engines[1].State().Total != 9 {
		t.Fatalf("b read %d of a, want 9", engines[1].State().Total)
	}
}

func TestAClusterFindsEnginesByIDAndTypeAndStepsThemInTheOrderTheyWereAdded(t *testing.T) {
	c, engines := toyCluster(t, 3)
	if got, ok := c.Get[*toyState](2); !ok || got != engines[1] {
		t.Fatal("Get did not find the engine with the ID 2")
	}
	if _, ok := c.Get[*toyState](9); ok {
		t.Fatal("Get found an engine that is not there")
	}
	if got := c.All[*toyState](); len(got) != 3 || got[0] != engines[0] || got[2] != engines[2] {
		t.Fatalf("All returned %v", got)
	}
	if stepper, ok := c.Find(3); !ok || stepper.StateID() != 3 {
		t.Fatal("Find did not find the engine with the ID 3")
	}
}

func TestACommandOfAnotherTypeOfStateAndANilCommandAreRefused(t *testing.T) {
	_, engines := toyCluster(t, 1)
	e := engines[0]
	var none sim.Command[*toyState]
	if e.Send(none) {
		t.Fatal("a nil command was taken")
	}
	if err := e.SendBaseCommand(otherCommand{}); err == nil {
		t.Fatal("a command of another type of state was taken")
	}
	if err := e.SendBaseCommand(toyCommand{N: 1}); err != nil {
		t.Fatalf("a command of its own type: %v", err)
	}
	for range 20 {
		if err := e.SendBaseCommand(toyCommand{}); err != nil {
			return // the queue is full: an error, not a panic
		}
	}
	t.Fatal("the queue of 16 did not fill")
}

// otherCommand is a command that is not one of the toy state.
type otherCommand struct{}

func (otherCommand) ID() sim.CommandID           { return 60009 }
func (otherCommand) Encode(*binary.Writer) error { return nil }

// BenchmarkEngineHash is the hash of an engine that holds messages that went and were not acked:
// what is taken at the period of the hash of the event log.
func BenchmarkEngineHash(b *testing.B) {
	c, engines := toyCluster(b, 2)
	for range 20 {
		engines[0].ScheduleAt(1, forwardAction{To: 2, N: 1})
	}
	if err := stepping.Step(c); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		hashOf(engines[0])
	}
}

// hashOf is the hash of an engine, from a new hasher.
func hashOf[S sim.State](e *engine.Engine[S]) uint64 { return e.Hash(sim.NewHasher()) }
