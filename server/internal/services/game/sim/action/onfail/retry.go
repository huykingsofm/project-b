package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Retry sends the message again to the same address.
type Retry struct{}

func (Retry) ID() message.OnFailID { return RetryID }

// Handle sends it again, unless the engine stops or the message was tried too often.
func (Retry) Handle(b message.Bouncer, m message.Message, r message.Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Again())
	}
}

func (Retry) Encode(*binarycodec.Writer) error { return nil }

func decodeRetry(
	*binarycodec.Reader, binarycodec.Version, message.MessageDecoder,
) (message.OnFail, error) {
	return Retry{}, nil
}
