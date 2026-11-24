package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Notify tells the actor that the message is about, with a text key and its arguments.
type Notify struct {
	Key  string
	Args []string
}

func (Notify) ID() sim.OnFailID { return NotifyID }

// Handle tells the actor of the sim.
func (f Notify) Handle(b sim.Bouncer, m sim.Message, _ sim.DeliveryFailure) {
	b.Tell(m.Subject(), f.Key, f.Args)
}

func (f Notify) Encode(w *binary.Writer) error {
	w.String(f.Key)
	w.Uvarint(uint64(len(f.Args)))
	for _, a := range f.Args {
		w.String(a)
	}
	return nil
}

func decodeNotify(
	r *binary.Reader, _ codec.Version, _ sim.MessageDecoder,
) (sim.OnFail, error) {
	f := Notify{Key: r.String()}
	n := r.Uvarint()
	if n > uint64(r.Left()) { // a string takes at least one byte
		return nil, binary.ErrShort
	}
	for range n {
		f.Args = append(f.Args, r.String())
	}
	return f, r.Err()
}
