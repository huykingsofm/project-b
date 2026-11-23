package replay

import (
	"bytes"
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// Kind names a kind of record by a number: the register below. A new kind takes the next number, at
// the end of the list, and a number is never changed or reused, because the log writes it.
type Kind uint8

// The kinds of record. Each is the Kind of one type of Body.
const (
	KindInbound Kind = iota + 1
	KindCommand
	KindConfig
	KindHash
	KindStableRead
	KindReturned
)

// Body is what a record holds besides the engine that made it and the tick it was made at. Each
// kind of record is a type of its own that holds only what it has, writes itself (Encode) and says
// whether another body is the same (Same); a function in this package reads it back, found by its
// kind.
type Body interface {
	Kind() Kind
	// Encode writes what the body holds, not its kind: encodeRecord writes the kind first.
	Encode(w *binarycodec.Writer) error
	// Same reports whether the other body is of the same kind and says the same thing.
	Same(other Body) bool
}

// Record is one entry of a log: who recorded it, at which tick, and what.
type Record struct {
	Engine sim.StateID
	Tick   uint64
	Body   Body
}

// Kind is the kind of the body of the record, or 0 when there is none.
func (r Record) Kind() Kind {
	if r.Body == nil {
		return 0
	}
	return r.Body.Kind()
}

// Inbound is a message that the engine took, whole.
type Inbound struct{ Message sim.Message }

func (Inbound) Kind() Kind { return KindInbound }

func (b Inbound) Encode(w *binarycodec.Writer) error { return sim.EncodeMessage(w, b.Message) }

func (b Inbound) Same(other Body) bool {
	o, ok := other.(Inbound)
	return ok && SameMessage(b.Message, o.Message)
}

func decodeInbound(
	c *sim.Codecs, r *binarycodec.Reader, version binarycodec.Version,
) (Body, error) {
	m, err := c.DecodeMessage(r, version)
	return Inbound{Message: m}, err
}

// Command is a command of a client that the engine took.
type Command struct{ Command sim.BaseCommand }

func (Command) Kind() Kind { return KindCommand }

func (b Command) Encode(w *binarycodec.Writer) error { return sim.EncodeCommand(w, b.Command) }

func (b Command) Same(other Body) bool {
	o, ok := other.(Command)
	if !ok {
		return false
	}
	var x, y binarycodec.Writer
	if sim.EncodeCommand(&x, b.Command) != nil || sim.EncodeCommand(&y, o.Command) != nil {
		return false
	}
	return bytes.Equal(x.B, y.B)
}

func decodeCommand(
	c *sim.Codecs, r *binarycodec.Reader, version binarycodec.Version,
) (Body, error) {
	cmd, err := c.DecodeCommand(r, version)
	return Command{Command: cmd}, err
}

// ConfigRead is the version of the configuration that the engine first read at the tick.
type ConfigRead struct{ Version uint64 }

func (ConfigRead) Kind() Kind { return KindConfig }

func (b ConfigRead) Encode(w *binarycodec.Writer) error {
	w.Uvarint(b.Version)
	return nil
}

func (b ConfigRead) Same(other Body) bool { o, ok := other.(ConfigRead); return ok && b == o }

func decodeConfigRead(_ *sim.Codecs, r *binarycodec.Reader, _ binarycodec.Version) (Body, error) {
	return ConfigRead{Version: r.Uvarint()}, r.Err()
}

// Hash is the hash of the state of the engine at the end of the tick.
type Hash struct{ Value uint64 }

func (Hash) Kind() Kind { return KindHash }

func (b Hash) Encode(w *binarycodec.Writer) error {
	w.Fixed64(b.Value)
	return nil
}

func (b Hash) Same(other Body) bool { o, ok := other.(Hash); return ok && b == o }

func decodeHash(_ *sim.Codecs, r *binarycodec.Reader, _ binarycodec.Version) (Body, error) {
	return Hash{Value: r.Fixed64()}, r.Err()
}

// StableRead says which copy of the state of another engine the engine read first at the tick: the
// tick the copy was taken at, or 0 when there was none.
type StableRead struct {
	Source   sim.StateID
	CopyTick uint64
}

func (StableRead) Kind() Kind { return KindStableRead }

func (b StableRead) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(b.Source))
	w.Uvarint(b.CopyTick)
	return nil
}

func (b StableRead) Same(other Body) bool { o, ok := other.(StableRead); return ok && b == o }

func decodeStableRead(_ *sim.Codecs, r *binarycodec.Reader, _ binarycodec.Version) (Body, error) {
	return StableRead{Source: r.Uint[sim.StateID](), CopyTick: r.Uvarint()}, r.Err()
}

// Returned says that a message the engine sent came back undelivered, and why: the header of the
// message, and the reason.
type Returned struct {
	Header sim.Header
	Reason sim.Reason
}

func (Returned) Kind() Kind { return KindReturned }

func (b Returned) Encode(w *binarycodec.Writer) error {
	b.Header.Encode(w)
	w.Uint8(uint8(b.Reason))
	return nil
}

func (b Returned) Same(other Body) bool { o, ok := other.(Returned); return ok && b == o }

func decodeReturned(_ *sim.Codecs, r *binarycodec.Reader, _ binarycodec.Version) (Body, error) {
	header := sim.DecodeHeader(r)
	return Returned{Header: header, Reason: sim.Reason(r.Uint8())}, r.Err()
}

// SameMessage reports whether two messages are the same one, by what they are written as: that is
// what the log holds, and it does not tell a nil list from an empty one.
func SameMessage(a, b sim.Message) bool {
	var x, y binarycodec.Writer
	if sim.EncodeMessage(&x, a) != nil || sim.EncodeMessage(&y, b) != nil {
		return false
	}
	return bytes.Equal(x.B, y.B)
}

// HashMessage is a number that depends on what a message is written as, so that a replay can keep
// that of each message it will compare later instead of the message. It is 0 for a message that
// cannot be written.
func HashMessage(m sim.Message) uint64 {
	var w binarycodec.Writer
	if sim.EncodeMessage(&w, m) != nil {
		return 0
	}
	h := sim.NewHasher()
	h.Bytes(w.B)
	return h.Sum()
}

// decoders is the table of the functions that read a body, by its kind. A test fails when a kind of
// the register has none.
var decoders = map[Kind]func(
	c *sim.Codecs, r *binarycodec.Reader, version binarycodec.Version,
) (Body, error){
	KindInbound:    decodeInbound,
	KindCommand:    decodeCommand,
	KindConfig:     decodeConfigRead,
	KindHash:       decodeHash,
	KindStableRead: decodeStableRead,
	KindReturned:   decodeReturned,
}

// payloadTick is the tick of the record that a payload holds.
func payloadTick(payload []byte) (uint64, error) {
	r := binarycodec.NewReader(payload)
	r.Uint8()
	tick := r.Uvarint()
	return tick, r.Err()
}

// encodeRecord writes a record: its kind, its tick and then its body. The engine is not in it: it
// is the engine of the log the record is written to.
func encodeRecord(w *binarycodec.Writer, r Record) error {
	if r.Body == nil {
		return fmt.Errorf("the record has no body")
	}
	w.Uint8(uint8(r.Body.Kind()))
	w.Uvarint(r.Tick)
	return r.Body.Encode(w)
}

// decodeRecord reads what encodeRecord wrote, of the data version given, for the log of an engine.
func decodeRecord(
	c *sim.Codecs, engine sim.StateID, version binarycodec.Version, payload []byte,
) (Record, error) {
	r := binarycodec.NewReader(payload)
	kind, tick := Kind(r.Uint8()), r.Uvarint()
	if err := r.Err(); err != nil {
		return Record{}, err
	}
	decode, ok := decoders[kind]
	if !ok {
		return Record{}, fmt.Errorf("the record kind %d is not one the replay log writes", kind)
	}
	body, err := decode(c, r, version)
	if err != nil {
		return Record{}, err
	}
	if r.Left() != 0 {
		return Record{}, fmt.Errorf("%d bytes are left after the record", r.Left())
	}
	return Record{Engine: engine, Tick: tick, Body: body}, nil
}
