package replay

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/command"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Kind says what a Record is.
type Kind uint8

// The kinds of record.
const (
	KindInbound Kind = iota + 1
	KindCommand
	KindConfig
	KindHash
	KindStableRead
)

// Record is one entry of a MemLog.
type Record struct {
	Kind    Kind
	Engine  action.Address
	Tick    uint64
	Header  action.Header
	Actions []action.ID
	Command command.Command
	Source  action.Address // for a KindStableRead, the engine whose copy was read
	// the version of a configuration, a hash, or the tick of a copy that was read
	Value uint64
}

// payloadTick is the tick of the record that a payload holds.
func payloadTick(payload []byte) (uint64, error) {
	r := binarycodec.NewReader(payload)
	r.Uint8()
	tick := r.Uvarint()
	return tick, r.Err()
}

// encodeRecord writes a record: its kind, its tick and then what its kind has. The engine is not in
// it: it is the engine of the log the record is written to.
func encodeRecord(w *binarycodec.Writer, r Record) error {
	w.Uint8(uint8(r.Kind))
	w.Uvarint(r.Tick)
	switch r.Kind {
	case KindInbound:
		r.Header.Encode(w)
		w.Uvarint(uint64(len(r.Actions)))
		for _, id := range r.Actions {
			w.Uvarint(uint64(id))
		}
	case KindCommand:
		return command.Encode(w, r.Command)
	case KindConfig:
		w.Uvarint(r.Value)
	case KindHash:
		w.Fixed64(r.Value)
	case KindStableRead:
		r.Source.Encode(w)
		w.Uvarint(r.Value)
	default:
		return fmt.Errorf("the record kind %d is not one the replay log writes", r.Kind)
	}
	return nil
}

// decodeRecord reads what encodeRecord wrote, of the data version given, for the log of an engine.
func decodeRecord(
	engine action.Address, version binarycodec.Version, payload []byte,
) (Record, error) {
	r := binarycodec.NewReader(payload)
	rec := Record{Kind: Kind(r.Uint8()), Engine: engine, Tick: r.Uvarint()}
	switch rec.Kind {
	case KindInbound:
		rec.Header = action.DecodeHeader(r, version)
		n := r.Uvarint()
		for i := uint64(0); i < n && r.Err() == nil; i++ {
			code := r.Uvarint()
			if r.Err() != nil {
				break
			}
			if code > 0xffff {
				return Record{}, fmt.Errorf("the action number %d is not one of an action", code)
			}
			rec.Actions = append(rec.Actions, action.ID(code))
		}
	case KindCommand:
		c, err := command.Decode(r, version)
		if err != nil {
			return Record{}, err
		}
		rec.Command = c
	case KindConfig:
		rec.Value = r.Uvarint()
	case KindHash:
		rec.Value = r.Fixed64()
	case KindStableRead:
		rec.Source = action.DecodeAddress(r, version)
		rec.Value = r.Uvarint()
	default:
		if r.Err() == nil {
			return Record{}, fmt.Errorf(
				"the record kind %d is not one the replay log writes", rec.Kind,
			)
		}
	}
	if err := r.Err(); err != nil {
		return Record{}, err
	}
	if r.Left() != 0 {
		return Record{}, fmt.Errorf("%d bytes are left after the record", r.Left())
	}
	return rec, nil
}
