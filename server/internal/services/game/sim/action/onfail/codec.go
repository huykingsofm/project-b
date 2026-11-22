package onfail

import (
	"fmt"

	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// decoders is the table of the functions that read a kind, by its ID, each next to the Encode that
// it reads. A test fails when an ID of the register has no decoder.
var decoders = map[message.OnFailID]func(
	r *binarycodec.Reader, version binarycodec.Version, decodeMessage message.MessageDecoder,
) (message.OnFail, error){
	RetryID:      decodeRetry,
	RedirectID:   decodeRedirect,
	CompensateID: decodeCompensate,
	NotifyID:     decodeNotify,
	EscalateID:   decodeEscalate,
	DropID:       decodeDrop,
}

// Encode writes an OnFail: its ID and then what it holds. It returns message.ErrNoCodec, and writes
// nothing, for a nil OnFail or one whose ID has no decoder.
func Encode(w *binarycodec.Writer, f message.OnFail) error {
	if f == nil {
		return fmt.Errorf("%w: nil OnFail", message.ErrNoCodec)
	}
	if _, ok := decoders[f.ID()]; !ok {
		return fmt.Errorf("%w: %T has the ID %d", message.ErrNoCodec, f, f.ID())
	}
	start := len(w.B)
	w.Uvarint(uint64(f.ID()))
	if err := f.Encode(w); err != nil {
		w.B = w.B[:start] // nothing of an OnFail that could not be written is left
		return err
	}
	return nil
}

// Decode reads what Encode wrote, of the data version given. A kind that holds a message reads it
// with decodeMessage.
func Decode(
	r *binarycodec.Reader, version binarycodec.Version, decodeMessage message.MessageDecoder,
) (message.OnFail, error) {
	code := r.Uvarint()
	if err := r.Err(); err != nil {
		return nil, err
	}
	decode, ok := decoders[message.OnFailID(code)]
	if !ok || code > 0xffff {
		return nil, fmt.Errorf("%w: OnFail %d", message.ErrUnknownCode, code)
	}
	return decode(r, version, decodeMessage)
}
