package onfail

import (
	"github.com/huykingsofm/project-b/platform/sim"
	"github.com/huykingsofm/project-b/shared/binarycodec"
)

// Drop forgets the message and counts it.
type Drop struct{}

func (Drop) ID() sim.OnFailID { return DropID }

// Handle drops the sim.
func (Drop) Handle(b sim.Bouncer, _ sim.Message, _ sim.Reason) { b.Drop() }

func (Drop) Encode(*binarycodec.Writer) error { return nil }

func decodeDrop(
	*binarycodec.Reader, binarycodec.Version, sim.MessageDecoder,
) (sim.OnFail, error) {
	return Drop{}, nil
}
