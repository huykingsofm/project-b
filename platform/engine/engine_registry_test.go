package engine_test

import (
	"testing"

	"github.com/huykingsofm/project-b/platform/engine"
	"github.com/huykingsofm/project-b/platform/eventlog"
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
	*sim.Context, *otherToy, *sim.Registry[*otherToy], *sim.Plan[*otherToy],
) {
}
func (a otherAction) Clone() sim.Action[*otherToy] { return a }
func (otherAction) Encode(*binary.Writer)          {}

func TestTheRegistryMakesTheEngineOfAStateOfAKnownTypeAndRefusesTheOthers(t *testing.T) {
	env := newToyEnv(t)
	handle, err := env.EngineRegistry.Make(
		env, 5, &toyState{Total: 4}, []sim.BaseAction{toyAction{N: 1}}, 0, 0, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	made := handle.(*engine.Engine[*toyState])
	if made.StateID() != 5 || made.Tick() != 0 || made.State().Total != 4 {
		t.Fatalf("the engine is %v at the tick %d", made.StateID(), made.Tick())
	}
	if _, err := env.EngineRegistry.Make(env, 6, &otherToy{}, nil, 0, 0, 0); err == nil {
		t.Fatal("an engine of a type that is not registered was made")
	}
	// an action of another state makes no engine, and registers none: a message to it bounces
	wrong := []sim.BaseAction{otherAction{}}
	_, err = env.EngineRegistry.Make(env, 7, &toyState{}, wrong, 0, 0, 0)
	if err == nil {
		t.Fatal("an action of another state was taken as an action of the state")
	}
	reason, ok := env.Router.Deliver(sim.NewActionMessage[*toyState](7, onfail.Drop{}, toyAction{}))
	if ok || reason != sim.DeliveryFailureNotStarted {
		t.Fatalf("a message to the engine that was not made: %v, %v", reason, ok)
	}
}

func TestAnEngineMadeAgainFromTheRecordOfItsStartIsARestoreAndWritesNoStart(t *testing.T) {
	env := newToyEnv(t)
	log := &eventlog.MemLog{}
	env.EventLog = log
	handle, err := env.EngineRegistry.Make(env, 5, &toyState{Total: 4}, nil, 3, 9, 2)
	if err != nil {
		t.Fatal(err)
	}
	made := handle.(*engine.Engine[*toyState])
	if err := made.Step(); err != nil {
		t.Fatal(err)
	}
	started, ok := log.ByEngine()[5][0].Body.(eventlog.Started)
	if !ok || started.Creator != 3 || started.CreatorTick != 9 || started.DirectiveID != 2 {
		t.Fatalf("the first record is %+v", log.ByEngine()[5][0])
	}
	again := newToyEnv(t)
	again.EventLog = &eventlog.MemLog{}
	restoredHandle, err := again.EngineRegistry.Restore(again, started.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restored := restoredHandle.(*engine.Engine[*toyState])
	if restored.Tick() != 0 || restored.StateID() != 5 {
		t.Fatalf("the engine made again is %v at the tick %d", restored.StateID(), restored.Tick())
	}
	if err := restored.Step(); err != nil {
		t.Fatal(err)
	}
	for _, r := range again.EventLog.(*eventlog.MemLog).ByEngine()[5] {
		if _, again := r.Body.(eventlog.Started); again {
			t.Fatal("the engine made again from the record of its start wrote it again")
		}
	}
}

func TestAnEngineOfATypeThatIsRegisteredTwiceOrNotAtAllCannotBeMadeAgainFromBytes(t *testing.T) {
	env := newToyEnv(t)
	codecs, engines, err := toyCodecs()
	if err != nil {
		t.Fatal(err)
	}
	if err := engines.Register(codecs, decodeToyState); err == nil {
		t.Fatal("a type of state was registered twice")
	}
	if _, err := engine.NewEngineRegistry().Restore(env, []byte{1, 5, 0}); err == nil {
		t.Fatal("an engine of a type that is not registered was restored")
	}
	if _, err := env.EngineRegistry.Restore(env, nil); err == nil {
		t.Fatal("nothing was restored as an engine")
	}
}
