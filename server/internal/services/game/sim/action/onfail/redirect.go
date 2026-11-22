package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Redirect sends the message to another engine of the same scope.
type Redirect struct{ To message.Address }

func (Redirect) ID() message.OnFailID { return RedirectID }

// Handle sends it, once more tried, to the other engine, unless the engine stops or the message was
// tried too often.
func (f Redirect) Handle(b message.Bouncer, m message.Message, r message.Reason) {
	if resendable(b, m, r) {
		b.Resend(m.Redirected(f.To).Again())
	}
}

func (f Redirect) Encode(w *binarycodec.Writer) error {
	f.To.Encode(w)
	return nil
}

func decodeRedirect(
	r *binarycodec.Reader, version binarycodec.Version, _ message.MessageDecoder,
) (message.OnFail, error) {
	return Redirect{To: message.DecodeAddress(r, version)}, r.Err()
}
