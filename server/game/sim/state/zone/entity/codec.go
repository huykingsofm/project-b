package entity

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// ErrNoCodec is returned for an entity that cannot be written: it is nil, or its kind has no
// decoder.
var ErrNoCodec = errors.New("zone entity: no codec for it")

// ErrUnknownKind is returned when bytes name a kind of entity that is not one a zone holds.
var ErrUnknownKind = errors.New("zone entity: the number is not one of a kind of entity")

// decoders is the table of the functions that read an entity of a zone, by its kind.
var decoders = map[state.KindID]func(
	r *binary.Reader, version codec.Version, owner sim.StateID,
) (state.Entity, error){
	state.ExitKindID: decodeExitEntity,
}

// Encode writes an entity: its kind and then what it is made of. It writes nothing, and returns
// ErrNoCodec, for a nil entity or one whose kind has no decoder.
func Encode(w *binary.Writer, e state.Entity) error {
	if e == nil {
		return fmt.Errorf("%w: nil", ErrNoCodec)
	}
	if _, ok := decoders[e.KindID()]; !ok {
		return fmt.Errorf("%w: %T has the kind %d", ErrNoCodec, e, e.KindID())
	}
	w.Uvarint(uint64(e.KindID()))
	e.Encode(w)
	return nil
}

// Decode reads what Encode wrote, of the data version given. An entity writes only the number its
// state gave it, so the ID of the state that holds it, which is not in the bytes, is given as
// owner.
func Decode(
	r *binary.Reader, version codec.Version, owner sim.StateID,
) (state.Entity, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := decoders[state.KindID(code)]
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKind, code)
	}
	return decode(r, version, owner)
}
