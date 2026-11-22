package sim

// ActionKey names a set of actions: the actions of one ID that one actor performs, or, with a nil
// Actor, all the actions of one ID. A plan keeps an index of the actions it holds by it (Find), the
// scheduler finds and cancels the actions that wait by it, and an action cancels through its
// registry by it (CancelWaiting). The actor is not written anywhere: a key is used in memory, to
// match actions, and never kept.
type ActionKey struct {
	Actor any
	ID    ActionID
}

// Names reports whether the action is one that the key names: of its ID, and, when the key has an
// actor, performed by that actor.
func (k ActionKey) Names(a BaseAction) bool {
	if a.ID() != k.ID {
		return false
	}
	if k.Actor == nil {
		return true
	}
	who, ok := a.(Acting)
	return ok && who.Actor() == k.Actor
}
