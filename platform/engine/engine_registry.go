package engine

import (
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// EngineRegistry knows, for each type of state, how to make the engine of a state of that type
// when only the cluster, which holds its engines without their state type, has the state or the
// bytes of a checkpoint: an engine that a request asked for, and an engine that a replay or a
// recovery makes again from the checkpoint it starts from. The game registers each type of its
// states once, with the decoder of the state.
type EngineRegistry struct {
	types map[sim.StateType]engineType
}

// NewEngineRegistry makes a registry that knows the configuration engine, which every cluster has,
// and nothing else.
func NewEngineRegistry() *EngineRegistry {
	r := &EngineRegistry{types: map[sim.StateType]engineType{}}
	r.add[*sim.ConfigState]()
	return r
}

// Register registers a type of state, as Codecs.RegisterState does (decode reads it, and the type
// is the one of S), and the way to make its engine. It is an error when the type or the Go type is
// already registered.
func (r *EngineRegistry) Register[S sim.State](
	codecs *sim.Codecs, decode func(r *binary.Reader, version codec.Version) (S, error),
) error {
	if err := codecs.RegisterState(decode); err != nil {
		return err
	}
	r.add[S]()
	return nil
}

// add keeps how to make the engines of the type of S.
func (r *EngineRegistry) add[S sim.State]() {
	r.types[sim.StateTypeOf[S]()] = engineType{
		make: func(
			env *Environment, id sim.StateID, st sim.State, start []sim.BaseAction,
			creator sim.StateID, creatorTick, directiveID uint64,
		) (EngineHandle, error) {
			typed, ok := st.(S)
			if !ok {
				return nil, fmt.Errorf("the state is a %T, which is not the one registered", st)
			}
			actions := make([]sim.Action[S], len(start))
			for i, base := range start {
				a, ok := base.(sim.Action[S])
				if !ok {
					return nil, fmt.Errorf("the action %T is not one of the state %T", base, st)
				}
				actions[i] = a
			}
			e := NewEngine(typed, id, env)
			for _, a := range actions {
				e.ScheduleAt(e.Tick()+1, a)
			}
			if err := e.markStarted(creator, creatorTick, directiveID); err != nil {
				e.Close()
				return nil, err
			}
			return e, nil
		},
		restore: func(env *Environment, checkpoint []byte) (EngineHandle, error) {
			c, err := DecodeEngineCheckpoint[S](checkpoint, env.Codecs)
			if err != nil {
				return nil, err
			}
			return c.Restore(env)
		},
	}
}

// Make makes the engine with the ID from a state and the actions it starts with, in the
// environment, new: at the tick of the state, registered with the router, with the record that
// begins its log ready. creator, creatorTick and directiveID say which engine asked for it, at
// which tick and with which request: 0, 0 and 0 for an engine that no engine asked for.
func (r *EngineRegistry) Make(
	env *Environment, id sim.StateID, st sim.State, start []sim.BaseAction,
	creator sim.StateID, creatorTick, directiveID uint64,
) (EngineHandle, error) {
	t, ok := r.types[st.Type()]
	if !ok {
		return nil, fmt.Errorf("the engines of the state type %d are not registered", st.Type())
	}
	return t.make(env, id, st, start, creator, creatorTick, directiveID)
}

// Restore makes the engine of a checkpoint, at its tick, in the environment; its state type is read
// from its ID. The engine in the record that begins a log (eventlog.Started.Snapshot) is made the
// same way.
func (r *EngineRegistry) Restore(env *Environment, checkpoint []byte) (EngineHandle, error) {
	rd := binary.NewReader(checkpoint)
	rd.Uvarint() // the version of the data
	id := rd.Uint[sim.StateID]()
	if err := rd.Err(); err != nil {
		return nil, err
	}
	t, ok := r.types[id.Type()]
	if !ok {
		return nil, fmt.Errorf("the engines of the state type %d are not registered", id.Type())
	}
	return t.restore(env, checkpoint)
}

// engineType is how the registry makes and restores the engines of one type of state.
type engineType struct {
	// make makes the engine with the ID from a state of the type and the actions that it starts
	// with, and the record that begins its log
	make func(
		env *Environment, id sim.StateID, st sim.State, start []sim.BaseAction,
		creator sim.StateID, creatorTick, directiveID uint64,
	) (EngineHandle, error)
	// restore makes the engine of a checkpoint (EngineCheckpoint.Encode)
	restore func(env *Environment, checkpoint []byte) (EngineHandle, error)
}
