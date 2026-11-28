package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/journal/eventlog"
	"github.com/huykingsofm/project-b/platform/runtime/engine"
	"github.com/huykingsofm/project-b/platform/runtime/testkit"
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/platform/sim/onfail"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// otherAction is an action of the state of another type.
type otherAction struct{}

func (otherAction) ID() sim.ActionID { return 60099 }
func (otherAction) Priority() int16  { return 0 }
func (otherAction) Actor() any       { return nil }
func (otherAction) Apply(
	*sim.Context, *otherToy, *sim.ActionRegistry[*otherToy], *sim.Plan[*otherToy],
) {
}
func (a otherAction) Clone() sim.Action[*otherToy] { return a }
func (otherAction) Encode(*binary.Writer)          {}

func TestTheRegistryMakesTheEngineOfAStateOfAKnownTypeAndRefusesTheOthers(t *testing.T) {
	env := testkit.NewToyEnv(t)
	handle, err := env.EngineRegistry.Make(
		env, 5, &testkit.ToyState{Total: 4}, []sim.BaseAction{testkit.ToyAction{N: 1}}, 0, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	made := handle.(*engine.Engine[*testkit.ToyState])
	// it has run its tick 1, which holds Init and runs no action: the actions it starts with are
	// for the tick 2
	if made.StateID() != 5 || made.Tick() != 1 || made.State().Total != 4 {
		t.Fatalf("the engine is %v at the tick %d", made.StateID(), made.Tick())
	}
	if reason, ok := env.Router.Deliver(
		sim.NewActionMessage[*testkit.ToyState](5, onfail.Drop{}, testkit.ToyAction{}),
	); ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("a message to the engine before it joined the router: %v, %v", reason, ok)
	}
	if _, err := env.EngineRegistry.Make(env, 6, &otherToy{}, nil, 0, 0, 0); err == nil {
		t.Fatal("an engine of a type that is not registered was made")
	}
	// an action of another state makes no engine, and registers none: a message to it bounces
	wrong := []sim.BaseAction{otherAction{}}
	_, err = env.EngineRegistry.Make(env, 7, &testkit.ToyState{}, wrong, 0, 0, 0)
	if err == nil {
		t.Fatal("an action of another state was taken as an action of the state")
	}
	reason, ok := env.Router.Deliver(
		sim.NewActionMessage[*testkit.ToyState](7, onfail.Drop{}, testkit.ToyAction{}),
	)
	if ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("a message to the engine that was not made: %v, %v", reason, ok)
	}
}

func TestAnEngineMadeAgainFromTheRecordOfItsStartIsARestoreAndWritesNoStart(t *testing.T) {
	env := testkit.NewToyEnv(t)
	log := &eventlog.MemLog{}
	env.EventLog = log
	toy := &testkit.ToyState{Total: 4}
	if _, err := env.EngineRegistry.Make(env, 5, toy, nil, 3, 9, 2); err != nil {
		t.Fatal(err)
	}
	started, ok := log.ByEngine()[5][0].Body.(eventlog.Init)
	if !ok || started.Creator != 3 || started.CreatorTick != 9 || started.DirectiveID != 2 {
		t.Fatalf("the first record is %+v", log.ByEngine()[5][0])
	}
	again := testkit.NewToyEnv(t)
	again.EventLog = &eventlog.MemLog{}
	restoredHandle, err := again.EngineRegistry.Restore(again, started.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restored := restoredHandle.(*engine.Engine[*testkit.ToyState])
	if restored.Tick() != 0 || restored.StateID() != 5 {
		// Init holds the engine as it was made, at the tick 0
		t.Fatalf("the engine made again is %v at the tick %d", restored.StateID(), restored.Tick())
	}
	if err := restored.Step(); err != nil {
		t.Fatal(err)
	}
	for _, r := range again.EventLog.(*eventlog.MemLog).ByEngine()[5] {
		if _, again := r.Body.(eventlog.Init); again {
			t.Fatal("the engine made again from the record of its start wrote it again")
		}
	}
}

func TestAnEngineOfATypeThatIsRegisteredTwiceOrNotAtAllCannotBeMadeAgainFromBytes(t *testing.T) {
	env := testkit.NewToyEnv(t)
	codecs, engines, err := testkit.ToyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	if err := engines.Register(codecs, testkit.DecodeToyState); err == nil {
		t.Fatal("a type of state was registered twice")
	}
	if _, err := engine.NewEngineRegistry().Restore(env, []byte{1, 5, 0}); err == nil {
		t.Fatal("an engine of a type that is not registered was restored")
	}
	if _, err := env.EngineRegistry.Restore(env, nil); err == nil {
		t.Fatal("nothing was restored as an engine")
	}
}
