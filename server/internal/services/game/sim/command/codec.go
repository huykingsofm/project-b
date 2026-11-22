package command

import (
	"errors"
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// ErrNoCodec is returned for a command that cannot be read back: its ID has no decoder.
var ErrNoCodec = errors.New("command: no codec for it")

// ErrUnknownCode is returned when bytes name a command that has no decoder,
var ErrUnknownCode = errors.New("command: the number is not one of a command")

// decoders is the table of the functions that read a command, by its ID: the one list of the
// commands besides the register of the IDs. Each is in the file of its command, next to the Encode
// that it reads. A test fails when an ID of the register has no decoder.
var decoders = map[ID]func(r *binarycodec.Reader, version binarycodec.Version) (Command, error){
	WorldChatPrivateSendCommandID:                  decodeWorldChatPrivateSendCommand,
	ZoneCharacterEnterCommandID:                    decodeZoneCharacterEnterCommand,
	ZoneChatSendCommandID:                          decodeZoneChatSendCommand,
	ZoneLayerCharacterMoveCommandID:                decodeZoneLayerCharacterMoveCommand,
	ZoneLayerCharacterNormalAttackTriggerCommandID: decodeZoneLayerCharacterNormalAttackTriggerCommand, //nolint:lll // two long names of one entry, which gofmt keeps on one line
	ZoneLayerChatSendCommandID:                     decodeZoneLayerChatSendCommand,
	ZoneLayerCharacterExitToCommandID:              decodeZoneLayerCharacterExitToCommand,
	ZoneLayerCharacterItemPickupCommandID:          decodeZoneLayerCharacterItemPickupCommand,
	ZoneLayerCharacterShieldGrantCommandID:         decodeZoneLayerCharacterShieldGrantCommand,
}

// Encode writes a command: its ID and then its fields. It returns ErrNoCodec, and writes nothing
// that can be read, for a command whose ID has no decoder: what is written must be readable.
func Encode(w *binarycodec.Writer, c Command) error {
	if c == nil {
		return fmt.Errorf("%w: nil", ErrNoCodec)
	}
	if _, ok := decoders[c.ID()]; !ok {
		return fmt.Errorf("%w: %T has the ID %d", ErrNoCodec, c, c.ID())
	}
	start := len(w.B)
	w.Uvarint(uint64(c.ID()))
	if err := c.Encode(w); err != nil {
		w.B = w.B[:start] // nothing of a command that could not be written is left
		return err
	}
	return nil
}

// Decode reads what Encode wrote, of the data version given. It returns the command as a value, or
// an error that wraps ErrUnknownCode (or zonelayer.ErrUnknownCooldownKey), or an error of the bytes
// (binarycodec.ErrShort).
func Decode(r *binarycodec.Reader, version binarycodec.Version) (Command, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := decoders[ID(code)]
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: %d", ErrUnknownCode, code)
	}
	return decode(r, version)
}
