package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Compensate sends the message that undoes what the first one set going: where it goes, its actions
// and its own OnFail. The chain ends at a kind that holds no message.
type Compensate struct{ With message.Message }

func (Compensate) ID() message.OnFailID { return CompensateID }

// Handle sends the message that undoes the first.
func (f Compensate) Handle(b message.Bouncer, _ message.Message, _ message.Reason) {
	b.Resend(f.With)
}

func (f Compensate) Encode(w *binarycodec.Writer) error {
	if f.With == nil {
		return message.ErrNoCodec
	}
	return f.With.Encode(w)
}

func decodeCompensate(
	r *binarycodec.Reader, version binarycodec.Version, decodeMessage message.MessageDecoder,
) (message.OnFail, error) {
	m, err := decodeMessage(r, version)
	return Compensate{With: m}, err
}
