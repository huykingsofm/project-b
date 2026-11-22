package action

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ErrNoCodec is returned for an action that cannot be read back: its ID has no decoder, or it has
// no Encode.
var ErrNoCodec = errors.New("action: no codec for it")

// ErrUnknownCode is returned when bytes name an action, a scope or an OnFail that is not one.
var ErrUnknownCode = errors.New("action: the number is not one of an action, a scope or an OnFail")

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

// EncodeAction writes an action: its ID and then its fields. It returns ErrNoCodec, and writes
// nothing that can be read, for an action that has no decoder or no Encode: what is written must be
// readable.
func EncodeAction(w *binarycodec.Writer, a Action) error {
	if a == nil {
		return fmt.Errorf("%w: nil", ErrNoCodec)
	}
	e, ok := a.(Encoder)
	if _, has := actionDecoders[a.ID()]; !ok || !has {
		return fmt.Errorf("%w: %T has the ID %d", ErrNoCodec, a, a.ID())
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
		return nil, fmt.Errorf("%w: action %d", ErrUnknownCode, code)
	}
	return decode(r, version)
}

// onFailDecoders is the table of the functions that read a kind of OnFail, by its ID, each in the
// file of its kind (on_fail.go) next to the Encode that it reads. A test fails when an ID of the
// register has no decoder. It is filled in by init because a message holds an OnFail that holds a
// message: written as the value of a variable, the table would depend on itself.
var onFailDecoders map[OnFailID]func(
	r *binarycodec.Reader, version binarycodec.Version, depth int,
) (OnFail, error)

func init() {
	onFailDecoders = map[OnFailID]func(
		r *binarycodec.Reader, version binarycodec.Version, depth int,
	) (OnFail, error){
		OnFailRetryID:      decodeOnFailRetry,
		OnFailRedirectID:   decodeOnFailRedirect,
		OnFailCompensateID: decodeOnFailCompensate,
		OnFailNotifyID:     decodeOnFailNotify,
		OnFailEscalateID:   decodeOnFailEscalate,
		OnFailDropID:       decodeOnFailDrop,
	}
}

// EncodeOnFail writes an OnFail: its ID and then what it holds. It returns ErrNoCodec, and writes
// nothing, for a nil OnFail or one whose ID has no decoder.
func EncodeOnFail(w *binarycodec.Writer, f OnFail) error {
	if f == nil {
		return fmt.Errorf("%w: nil OnFail", ErrNoCodec)
	}
	if _, ok := onFailDecoders[f.ID()]; !ok {
		return fmt.Errorf("%w: %T has the ID %d", ErrNoCodec, f, f.ID())
	}
	start := len(w.B)
	w.Uvarint(uint64(f.ID()))
	if err := f.Encode(w); err != nil {
		w.B = w.B[:start] // nothing of an OnFail that could not be written is left
		return err
	}
	return nil
}

func decodeOnFail(r *binarycodec.Reader, version binarycodec.Version, depth int) (OnFail, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := onFailDecoders[OnFailID(code)]
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: OnFail %d", ErrUnknownCode, code)
	}
	return decode(r, version, depth)
}

// messageDecoders is the table of the functions that read the rest of a message, by the scope of
// the engine it is for. A test fails when a scope has none.
var messageDecoders = map[Scope]func(
	to Address, r *binarycodec.Reader, version binarycodec.Version, depth int,
) (Message, error){
	ScopeWorld:     decodeScopedMessage[WorldAction],
	ScopeZone:      decodeScopedMessage[ZoneAction],
	ScopeZoneLayer: decodeScopedMessage[ZoneLayerAction],
}

// EncodeMessage writes a message whole: where it goes, who sent it, how many times it was tried,
// what a failed delivery means, and its actions. A message that holds an action with no codec is
// not written.
func EncodeMessage(w *binarycodec.Writer, m Message) error {
	if m == nil {
		return fmt.Errorf("%w: nil message", ErrNoCodec)
	}
	start := len(w.B)
	m.Dest().Encode(w)
	if err := m.encodeBody(w); err != nil {
		w.B = w.B[:start] // nothing of a message that could not be written is left
		return err
	}
	return nil
}

// DecodeMessage reads what EncodeMessage wrote, of the data version given.
func DecodeMessage(r *binarycodec.Reader, version binarycodec.Version) (Message, error) {
	return decodeMessage(r, version, 0)
}

func decodeMessage(r *binarycodec.Reader, version binarycodec.Version, depth int) (Message, error) {
	if depth > 8 {
		return nil, fmt.Errorf(
			"%w: messages inside messages go deeper than 8", binarycodec.ErrBadValue,
		)
	}
	to := DecodeAddress(r, version)
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := messageDecoders[to.Scope]
	if !ok {
		return nil, fmt.Errorf("%w: scope %d", ErrUnknownCode, to.Scope)
	}
	return decode(to, r, version, depth)
}

func (m *ScopedMessage[A]) encodeBody(w *binarycodec.Writer) error {
	m.Header.Encode(w)
	w.Uvarint(uint64(m.attempts))
	if err := EncodeOnFail(w, m.onFail); err != nil {
		return err
	}
	w.Uvarint(uint64(len(m.Actions)))
	for _, a := range m.Actions {
		if err := EncodeAction(w, a); err != nil {
			return err
		}
	}
	return nil
}

func decodeScopedMessage[A Action](
	to Address, r *binarycodec.Reader, version binarycodec.Version, depth int,
) (Message, error) {
	header := DecodeHeader(r, version)
	attempts := r.Uvarint()
	onFail, err := decodeOnFail(r, version, depth)
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
				"%w: %T is not an action of the scope %s", ErrUnknownCode, decoded, to.Scope,
			)
		}
		actions = append(actions, a)
	}
	m := newScopedMessage(to, onFail, actions)
	m.Header = header
	m.attempts = int(attempts)
	return m, r.Err()
}
