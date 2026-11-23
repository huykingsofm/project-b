package sim

import "github.com/huykingsofm/project-b/shared/ids"

// StateDirectory says what a state ID is: a state that is live, one that was given and is in the
// grave or free (it was removed), or one that was never given. Where states come and go, the state
// that gives their IDs is the directory, and a router asks it why a message has no engine to go to.
// What it answers for a state that exists is Live, whether or not its engine runs.
type StateDirectory interface {
	StateStatus(id StateID) ids.Status
}
