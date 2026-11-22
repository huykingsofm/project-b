package action

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/onfail"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Encoder is what an action has to be written: its fields, without its ID.
type Encoder interface {
	Encode(w *binarycodec.Writer)
}

// actionDecoders is the table of the functions that read an action, by its ID: the one list of the
// actions besides the register of the IDs. Each is in the file of its action, next to the Encode
// that it reads. A test fails when an ID of the register has no decoder.
var actionDecoders = map[ID]func(
	r *binarycodec.Reader, version binarycodec.Version,
) (Action, error){
	WorldChatPrivateSendActionID:                  decodeWorldChatPrivateSendAction,
	WorldCharacterItemAddActionID:                 decodeWorldCharacterItemAddAction,
	ZoneChatSendActionID:                          decodeZoneChatSendAction,
	ZoneCharacterEnterActionID:                    decodeZoneCharacterEnterAction,
	ZoneLayerChatSendActionID:                     decodeZoneLayerChatSendAction,
	ZoneLayerCharacterEnterActionID:               decodeZoneLayerCharacterEnterAction,
	ZoneLayerCharacterMoveActionID:                decodeZoneLayerCharacterMoveAction,
	ZoneLayerCharacterNormalAttackTriggerActionID: decodeZoneLayerCharacterNormalAttackTriggerAction, //nolint:lll // two long names of one entry, which gofmt keeps on one line
	ZoneLayerCharacterDamageActionID:              decodeZoneLayerCharacterDamageAction,
	ZoneLayerCharacterShieldGrantActionID:         decodeZoneLayerCharacterShieldGrantAction,
	ZoneLayerCharacterRegenActionID:               decodeZoneLayerCharacterRegenAction,
	ZoneLayerCharacterExitToActionID:              decodeZoneLayerCharacterExitToAction,
	ZoneLayerCharacterItemPickupActionID:          decodeZoneLayerCharacterItemPickupAction,
	ZoneLayerItemEntityAddActionID:                decodeZoneLayerItemEntityAddAction,
	ZoneLayerItemEntityRemoveActionID:             decodeZoneLayerItemEntityRemoveAction,
}

// EncodeAction writes an action: its ID and then its fields. It returns message.ErrNoCodec, and
// writes
// nothing that can be read, for an action that has no decoder or no Encode: what is written must be
// readable.
func EncodeAction(w *binarycodec.Writer, a Action) error {
	if a == nil {
		return fmt.Errorf("%w: nil", message.ErrNoCodec)
	}
	e, ok := a.(Encoder)
	if _, has := actionDecoders[a.ID()]; !ok || !has {
		return fmt.Errorf("%w: %T has the ID %d", message.ErrNoCodec, a, a.ID())
	}
	w.Uvarint(uint64(a.ID()))
	e.Encode(w)
	return nil
}

// DecodeAction reads what EncodeAction wrote, of the data version given.
func DecodeAction(r *binarycodec.Reader, version binarycodec.Version) (Action, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := actionDecoders[ID(code)]
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: action %d", message.ErrUnknownCode, code)
	}
	return decode(r, version)
}

// messageDecoders is the table of the functions that read the rest of a message, by the scope of
// the engine it is for. A test fails when a scope has none.
var messageDecoders = map[message.Scope]func(
	to message.Address, r *binarycodec.Reader, version binarycodec.Version,
	decodeNested message.MessageDecoder,
) (message.Message, error){
	message.ScopeWorld:     decodeScopedMessage[WorldAction],
	message.ScopeZone:      decodeScopedMessage[ZoneAction],
	message.ScopeZoneLayer: decodeScopedMessage[ZoneLayerAction],
}

// EncodeMessage writes a message whole: where it goes, who sent it, how many times it was tried,
// what a failed delivery means, and its actions. A message that holds an action with no codec is
// not written.
func EncodeMessage(w *binarycodec.Writer, m message.Message) error {
	if m == nil {
		return fmt.Errorf("%w: nil message", message.ErrNoCodec)
	}
	if nesting(m) > 8 { // what DecodeMessage reads: what is written must be readable
		return fmt.Errorf(
			"%w: messages inside messages go deeper than 8", binarycodec.ErrBadValue,
		)
	}
	return m.Encode(w)
}

// nesting is how many messages are inside one another through the OnFail of the messages: 0 for a
// message that holds none.
func nesting(m message.Message) int {
	if c, ok := m.OnFail().(onfail.Compensate); ok && c.With != nil {
		return 1 + nesting(c.With)
	}
	return 0
}

// DecodeMessage reads what EncodeMessage wrote, of the data version given.
func DecodeMessage(
	r *binarycodec.Reader, version binarycodec.Version,
) (message.Message, error) {
	return decodeMessage(r, version, 0)
}

func decodeMessage(
	r *binarycodec.Reader, version binarycodec.Version, depth int,
) (message.Message, error) {
	if depth > 8 {
		return nil, fmt.Errorf(
			"%w: messages inside messages go deeper than 8", binarycodec.ErrBadValue,
		)
	}
	to := message.DecodeAddress(r, version)
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := messageDecoders[to.Scope]
	if !ok {
		return nil, fmt.Errorf("%w: scope %d", message.ErrUnknownCode, to.Scope)
	}
	return decode(to, r, version, func(
		r *binarycodec.Reader, version binarycodec.Version,
	) (message.Message, error) {
		return decodeMessage(r, version, depth+1)
	})
}

// Encode writes the message whole, and nothing of it when it cannot be written.
func (m *ScopedMessage[A]) Encode(w *binarycodec.Writer) error {
	start := len(w.B)
	m.To.Encode(w)
	m.Header.Encode(w)
	w.Uvarint(uint64(m.attempts))
	if err := onfail.Encode(w, m.onFail); err != nil {
		w.B = w.B[:start]
		return err
	}
	w.Uvarint(uint64(len(m.Actions)))
	for _, a := range m.Actions {
		if err := EncodeAction(w, a); err != nil {
			w.B = w.B[:start]
			return err
		}
	}
	return nil
}

// decodeScopedMessage reads the rest of a message for an engine of the scope of A. A message inside
// its OnFail is read with decodeNested.
func decodeScopedMessage[A Action](
	to message.Address, r *binarycodec.Reader, version binarycodec.Version,
	decodeNested message.MessageDecoder,
) (message.Message, error) {
	header := message.DecodeHeader(r, version)
	attempts := r.Uvarint()
	onFail, err := onfail.Decode(r, version, decodeNested)
	if err != nil {
		return nil, err
	}
	n := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if n > uint64(r.Left()) { // an action takes at least one byte
		return nil, fmt.Errorf("%w: %d actions", binarycodec.ErrShort, n)
	}
	var actions []A // nil, not empty, when there is none, as a message made in code has it
	for range n {
		decoded, err := DecodeAction(r, version)
		if err != nil {
			return nil, err
		}
		a, ok := decoded.(A)
		if !ok {
			return nil, fmt.Errorf(
				"%w: %T is not an action of the scope %s",
				message.ErrUnknownCode, decoded, to.Scope,
			)
		}
		actions = append(actions, a)
	}
	m := newScopedMessage(to, onFail, actions)
	m.Header = header
	m.attempts = int(attempts)
	return m, r.Err()
}
