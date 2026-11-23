package replay

import (
	"bytes"
	"fmt"

	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
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
	KindRetired
)

// Body is what a record holds besides the engine that made it and the tick it was made at. Each
// kind of record is a type of its own that holds only what it has, writes itself (Encode) and says
// whether another body is the same (Same); a function in this package reads it back, found by its
// kind.
type Body interface {
	Kind() Kind
	// Encode writes what the body holds, not its kind: encodeRecord writes the kind first.
	Encode(w *binary.Writer) error
	// Same reports whether the other body is of the same kind and says the same thing.
	Same(other Body) bool
}

// Record is one entry of a log: who recorded it, at which tick, and what.
type Record struct {
	Engine sim.StateID
	Tick   uint64
	Body   Body
}

// Dependent is what a body says when what an engine recorded makes it depend on another engine.
type Dependent interface {
	// Dependency is the engine and the tick that the body depends on, and false when it depends on
	// none.
	Dependency() (sim.StateDependency, bool)
}

// Dependency is what the record makes the engine depend on, or false when it makes it depend on
// nothing: a message depends on the tick at which its sender sent it, and a read of a copy of the
// state of another engine on the tick of the copy.
func (r Record) Dependency() (sim.StateDependency, bool) {
	if d, ok := r.Body.(Dependent); ok {
		return d.Dependency()
	}
	return sim.StateDependency{}, false
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

func (b Inbound) Encode(w *binary.Writer) error { return sim.EncodeMessage(w, b.Message) }

// Dependency is the sender of the message and the tick at which it sent it; a message that was not
// stamped has none.
func (b Inbound) Dependency() (sim.StateDependency, bool) {
	h := b.Message.Head()
	return sim.StateDependency{Source: h.From, Tick: h.Tick}, h.Tick > 0
}

func (b Inbound) Same(other Body) bool {
	o, ok := other.(Inbound)
	return ok && SameMessage(b.Message, o.Message)
}

func decodeInbound(
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (Body, error) {
	m, err := c.DecodeMessage(r, version)
	return Inbound{Message: m}, err
}

// Command is a command of a client that the engine took.
type Command struct{ Command sim.BaseCommand }

func (Command) Kind() Kind { return KindCommand }

func (b Command) Encode(w *binary.Writer) error { return sim.EncodeCommand(w, b.Command) }

func (b Command) Same(other Body) bool {
	o, ok := other.(Command)
	if !ok {
		return false
	}
	var x, y binary.Writer
	if sim.EncodeCommand(&x, b.Command) != nil || sim.EncodeCommand(&y, o.Command) != nil {
		return false
	}
	return bytes.Equal(x.B, y.B)
}

func decodeCommand(
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (Body, error) {
	cmd, err := c.DecodeCommand(r, version)
	return Command{Command: cmd}, err
}

// ConfigRead is the version of the configuration that the engine first read at the tick.
type ConfigRead struct{ Version uint64 }

func (ConfigRead) Kind() Kind { return KindConfig }

func (b ConfigRead) Encode(w *binary.Writer) error {
	w.Uvarint(b.Version)
	return nil
}

func (b ConfigRead) Same(other Body) bool { o, ok := other.(ConfigRead); return ok && b == o }

func decodeConfigRead(_ *sim.Codecs, r *binary.Reader, _ codec.Version) (Body, error) {
	return ConfigRead{Version: r.Uvarint()}, r.Err()
}

// Hash is the hash of the state of the engine at the end of the tick.
type Hash struct{ Value uint64 }

func (Hash) Kind() Kind { return KindHash }

func (b Hash) Encode(w *binary.Writer) error {
	w.Fixed64(b.Value)
	return nil
}

func (b Hash) Same(other Body) bool { o, ok := other.(Hash); return ok && b == o }

func decodeHash(_ *sim.Codecs, r *binary.Reader, _ codec.Version) (Body, error) {
	return Hash{Value: r.Fixed64()}, r.Err()
}

// StableRead says which copy of the state of another engine the engine read first at the tick: the
// tick the copy was taken at, or 0 when there was none.
type StableRead struct {
	Source   sim.StateID
	CopyTick uint64
}

func (StableRead) Kind() Kind { return KindStableRead }

// Dependency is the engine that was read and the tick of the copy; a read that found no copy (a
// tick of 0) depends on nothing.
func (b StableRead) Dependency() (sim.StateDependency, bool) {
	return sim.StateDependency{Source: b.Source, Tick: b.CopyTick}, b.CopyTick > 0
}

func (b StableRead) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(b.Source))
	w.Uvarint(b.CopyTick)
	return nil
}

func (b StableRead) Same(other Body) bool { o, ok := other.(StableRead); return ok && b == o }

func decodeStableRead(_ *sim.Codecs, r *binary.Reader, _ codec.Version) (Body, error) {
	return StableRead{Source: r.Uint[sim.StateID](), CopyTick: r.Uvarint()}, r.Err()
}

// Returned says that a message the engine sent came back undelivered, and why: the header of the
// message, and the reason.
type Returned struct {
	Header sim.Header
	Reason sim.Reason
}

func (Returned) Kind() Kind { return KindReturned }

func (b Returned) Encode(w *binary.Writer) error {
	b.Header.Encode(w)
	w.Uint8(uint8(b.Reason))
	return nil
}

func (b Returned) Same(other Body) bool { o, ok := other.(Returned); return ok && b == o }

func decodeReturned(_ *sim.Codecs, r *binary.Reader, _ codec.Version) (Body, error) {
	header := sim.DecodeHeader(r)
	return Returned{Header: header, Reason: sim.Reason(r.Uint8())}, r.Err()
}

// Retired says that the engine was removed for good: it is the only record of the last frame of
// its log, which comes after the last tick it ran. A log that ends with it is of an engine that
// does not go on, and a state ID that is given again goes on counting ticks after it.
type Retired struct{}

func (Retired) Kind() Kind { return KindRetired }

func (Retired) Encode(*binary.Writer) error { return nil }

func (Retired) Same(other Body) bool { _, ok := other.(Retired); return ok }

func decodeRetired(_ *sim.Codecs, r *binary.Reader, _ codec.Version) (Body, error) {
	return Retired{}, r.Err()
}

// SameMessage reports whether two messages are the same one, by what they are written as: that is
// what the log holds, and it does not tell a nil list from an empty one.
func SameMessage(a, b sim.Message) bool {
	var x, y binary.Writer
	if sim.EncodeMessage(&x, a) != nil || sim.EncodeMessage(&y, b) != nil {
		return false
	}
	return bytes.Equal(x.B, y.B)
}

// HashMessage is a number that depends on what a message is written as, so that a replay can keep
// that of each message it will compare later instead of the message. It is 0 for a message that
// cannot be written.
func HashMessage(m sim.Message) uint64 {
	var w binary.Writer
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
	c *sim.Codecs, r *binary.Reader, version codec.Version,
) (Body, error){
	KindInbound:    decodeInbound,
	KindCommand:    decodeCommand,
	KindConfig:     decodeConfigRead,
	KindHash:       decodeHash,
	KindStableRead: decodeStableRead,
	KindReturned:   decodeReturned,
	KindRetired:    decodeRetired,
}

// payloadTick is the tick of the frame that a payload holds: a frame starts with its tick.
func payloadTick(payload []byte) (uint64, error) {
	r := binary.NewReader(payload)
	tick := r.Uvarint()
	return tick, r.Err()
}

// encodeRecord writes a record: its kind and then its body. The tick is the one of the frame the
// record is in, and the engine the one of the log it is written to.
func encodeRecord(w *binary.Writer, body Body) error {
	if body == nil {
		return fmt.Errorf("the record has no body")
	}
	w.Uint8(uint8(body.Kind()))
	return body.Encode(w)
}

// decodeRecord reads what encodeRecord wrote, of the data version given, for the log of an engine
// and a frame of a tick.
func decodeRecord(
	c *sim.Codecs, engine sim.StateID, version codec.Version, tick uint64, data []byte,
) (Record, error) {
	r := binary.NewReader(data)
	kind := Kind(r.Uint8())
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
