package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Retry sends the message again to the same address.
type Retry struct{}

func (Retry) ID() sim.OnFailID { return RetryID }

// Handle sends it again, unless the engine stops, the message was tried too often, or the engine
// it is for was retired, which sending it again would not change: it is dropped then, and it was
// asked once what to do.
func (Retry) Handle(b sim.Bouncer, m sim.Message, r sim.DeliveryFailure) {
	if r == sim.DeliveryFailureRetired {
		b.Drop()
		return
	}
	if resendable(b, m, r) {
		b.Resend(m.Again())
	}
}

// Clone returns the kind itself: it holds only values.
func (f Retry) Clone() sim.OnFail { return f }

func (Retry) Encode(*binary.Writer) error { return nil }

func decodeRetry(
	*binary.Reader, codec.Version, sim.MessageDecoder,
) (sim.OnFail, error) {
	return Retry{}, nil
}
