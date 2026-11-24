package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/codec"
	"github.com/huykingsofm/project-b/shared/codec/binary"
)

// Drop forgets the message and counts it.
type Drop struct{}

func (Drop) ID() sim.OnFailID { return DropID }

// Handle drops the sim.
func (Drop) Handle(b sim.Bouncer, _ sim.Message, _ sim.DeliveryFailure) { b.Drop() }

func (Drop) Encode(*binary.Writer) error { return nil }

func decodeDrop(
	*binary.Reader, codec.Version, sim.MessageDecoder,
) (sim.OnFail, error) {
	return Drop{}, nil
}
