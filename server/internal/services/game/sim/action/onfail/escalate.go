package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Escalate gives the message to the engine at the address to decide.
type Escalate struct{ To message.Address }

func (Escalate) ID() message.OnFailID { return EscalateID }

// Handle gives it to the other engine, unless the engine stops.
func (f Escalate) Handle(b message.Bouncer, m message.Message, _ message.Reason) {
	if b.Stopping() {
		b.Drop()
		return
	}
	b.Resend(m.Redirected(f.To))
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
