package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// Redirect sends the message to another engine.
type Redirect struct{ To sim.StateID }

func (Redirect) ID() sim.OnFailID { return RedirectID }

// Handle sends it, once more tried, to the other engine, unless the engine stops or the message was
// tried too often.
func (f Redirect) Handle(b sim.Bouncer, m sim.Message, r sim.Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Redirected(f.To).Again())
	}
}

func (f Redirect) Encode(w *binarycodec.Writer) error {
	w.Uvarint(uint64(f.To))
	return nil
}

func decodeRedirect(
	r *binarycodec.Reader, version binarycodec.Version, _ sim.MessageDecoder,
) (sim.OnFail, error) {
	return Redirect{To: sim.StateID(r.Uvarint())}, r.Err()
}
