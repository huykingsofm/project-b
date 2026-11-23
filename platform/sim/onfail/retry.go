package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Retry sends the message again to the same address.
type Retry struct{}

func (Retry) ID() sim.OnFailID { return RetryID }

// Handle sends it again, unless the engine stops or the message was tried too often.
func (Retry) Handle(b sim.Bouncer, m sim.Message, r sim.Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Again())
	}
}

func (Retry) Encode(*binary.Writer) error { return nil }

func decodeRetry(
	*binary.Reader, codec.Version, sim.MessageDecoder,
) (sim.OnFail, error) {
	return Retry{}, nil
}
