package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Escalate gives the message to the engine at the address to decide.
type Escalate struct{ To sim.StateID }

func (Escalate) ID() sim.OnFailID { return EscalateID }

// Handle gives it to the other engine, once more tried, unless the engine stops or the message was
// tried too often: two engines that escalate to each other would send it back and forth for ever.
func (f Escalate) Handle(b sim.Bouncer, m sim.Message, r sim.DeliveryFailure) {
	if resendable(b, m, r) {
		b.Resend(m.Redirected(f.To).Again())
	}
}

// Clone returns the kind itself: it holds only values.
func (f Escalate) Clone() sim.OnFail { return f }

func (f Escalate) Encode(w *binary.Writer) error {
	w.Uvarint(uint64(f.To))
	return nil
}

func decodeEscalate(
	r *binary.Reader, version codec.Version, _ sim.MessageDecoder,
) (sim.OnFail, error) {
	return Escalate{To: r.Uint[sim.StateID]()}, r.Err()
}
