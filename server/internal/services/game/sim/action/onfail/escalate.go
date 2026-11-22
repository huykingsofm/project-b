package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Escalate gives the message to the engine at the address to decide.
type Escalate struct{ To message.Address }

func (Escalate) ID() message.OnFailID { return EscalateID }

// Handle gives it to the other engine, once more tried, unless the engine stops or the message was
// tried too often: two engines that escalate to each other would send it back and forth for ever.
func (f Escalate) Handle(b message.Bouncer, m message.Message, r message.Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Redirected(f.To).Again())
	}
}

func (f Escalate) Encode(w *binarycodec.Writer) error {
	f.To.Encode(w)
	return nil
}

func decodeEscalate(
	r *binarycodec.Reader, version binarycodec.Version, _ message.MessageDecoder,
) (message.OnFail, error) {
	return Escalate{To: message.DecodeAddress(r, version)}, r.Err()
}
