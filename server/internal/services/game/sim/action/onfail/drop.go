package onfail

import (
	"github.com/huykingsofm/project-b/server/internal/services/game/sim/action/message"
	"github.com/huykingsofm/project-b/server/internal/shared/binarycodec"
)

// Drop forgets the message and counts it.
type Drop struct{}

func (Drop) ID() message.OnFailID { return DropID }

// Handle drops the message.
func (Drop) Handle(b message.Bouncer, _ message.Message, _ message.Reason) { b.Drop() }

func (Drop) Encode(*binarycodec.Writer) error { return nil }

func decodeDrop(
	*binarycodec.Reader, binarycodec.Version, message.MessageDecoder,
) (message.OnFail, error) {
	return Drop{}, nil
}
