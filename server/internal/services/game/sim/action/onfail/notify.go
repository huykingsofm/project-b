package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Notify tells the actor that the message is about, with a text key and its arguments.
type Notify struct {
	Key  string
	Args []string
}

func (Notify) ID() message.OnFailID { return NotifyID }

// Handle tells the actor of the message.
func (f Notify) Handle(b message.Bouncer, m message.Message, _ message.Reason) {
	b.Tell(m.Subject(), f.Key, f.Args)
}

func (f Notify) Encode(w *binarycodec.Writer) error {
	w.String(f.Key)
	w.Uvarint(uint64(len(f.Args)))
	for _, a := range f.Args {
		w.String(a)
	}
	return nil
}

func decodeNotify(
	r *binarycodec.Reader, _ binarycodec.Version, _ message.MessageDecoder,
) (message.OnFail, error) {
	f := Notify{Key: r.String()}
	n := r.Uvarint()
	if n > uint64(r.Left()) { // a string takes at least one byte
		return nil, binarycodec.ErrShort
	}
	for range n {
		f.Args = append(f.Args, r.String())
	}
	return f, r.Err()
}
