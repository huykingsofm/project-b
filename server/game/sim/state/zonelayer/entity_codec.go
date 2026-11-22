package zonelayer

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// ErrNoEntityCodec is returned for an entity that cannot be written: it is nil, or its kind has no
// decoder, so what is written could not be read back.
var ErrNoEntityCodec = errors.New("entity: no codec for it")

// ErrUnknownEntityKind is returned when bytes name a kind of entity that is not one.
var ErrUnknownEntityKind = errors.New("entity: the number is not one of a kind of entity")

// decoders is the table of the functions that read an entity, by its kind, each next to the Encode
// that it reads. A test fails when a kind of the register has none.
var decoders = map[KindID]func(r *binarycodec.Reader, version binarycodec.Version) (Entity, error){
	CharacterKindID: decodeCharacterEntity,
	ItemKindID:      decodeItemEntity,
}

// EncodeEntity writes an entity: its kind and then what it is made of. It writes nothing, and
// returns ErrNoEntityCodec, for a nil entity or one whose kind has no decoder.
func EncodeEntity(w *binarycodec.Writer, e Entity) error {
	if e == nil {
		return fmt.Errorf("%w: nil", ErrNoEntityCodec)
	}
	if _, ok := decoders[e.KindID()]; !ok {
		return fmt.Errorf("%w: %T has the kind %d", ErrNoEntityCodec, e, e.KindID())
	}
	w.Uvarint(uint64(e.KindID()))
	e.Encode(w)
	return nil
}

// DecodeEntity reads what Encode wrote, of the data version given.
func DecodeEntity(r *binarycodec.Reader, version binarycodec.Version) (Entity, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := decoders[KindID(code)]
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: %d", ErrUnknownEntityKind, code)
	}
	return decode(r, version)
}
