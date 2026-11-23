package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// Escalate gives the message to the engine at the address to decide.
type Escalate struct{ To sim.StateID }

func (Escalate) ID() sim.OnFailID { return EscalateID }

// Handle gives it to the other engine, once more tried, unless the engine stops or the message was
// tried too often: two engines that escalate to each other would send it back and forth for ever.
func (f Escalate) Handle(b sim.Bouncer, m sim.Message, r sim.Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Redirected(f.To).Again())
	}
}

func (f Escalate) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(f.To))
	return nil
}

func decodeEscalate(
	r *binarycodec.Reader, version binarycodec.Version, _ sim.MessageDecoder,
) (sim.OnFail, error) {
	return Escalate{To: r.Uint[sim.StateID]()}, r.Err()
}
