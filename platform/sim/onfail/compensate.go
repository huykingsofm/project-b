package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Compensate sends the message that undoes what the first one set going: where it goes, its actions
// and its own OnFail. The chain ends at a kind that holds no sim.
type Compensate struct{ With sim.Message }

func (Compensate) ID() sim.OnFailID { return CompensateID }

// Handle sends the message that undoes the first.
func (f Compensate) Handle(b sim.Bouncer, _ sim.Message, _ sim.DeliveryFailure) {
	b.Resend(f.With)
}

func (f Compensate) Nested() sim.Message { return f.With }

// Clone returns a copy that shares nothing with the kind.
func (f Compensate) Clone() sim.OnFail {
	if f.With == nil {
		return f
	}
	return Compensate{With: f.With.Clone()}
}

func (f Compensate) Encode(w *binary.Writer) error {
	if f.With == nil {
		return sim.ErrNoCodec
	}
	return f.With.Encode(w)
}

func decodeCompensate(
	r *binary.Reader, version codec.Version, decodeMessage sim.MessageDecoder,
) (sim.OnFail, error) {
	m, err := decodeMessage(r, version)
	return Compensate{With: m}, err
}
