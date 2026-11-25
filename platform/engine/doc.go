// Package engine runs the engines of a game that runs in ticks. Each one is a goroutine that, at
// every tick, takes what was sent to it, runs it as a plan of actions over its state, tells its
// clients what changed, publishes a copy of its state for the other engines to read, and sends the
// messages that its actions asked for.
//
// Everything about the actions and the state is in package sim; this package is what makes them
// run, and what connects the engines: the router that carries the messages, the store of published
// states, the bouncer that runs what a message says a failed delivery means, and the event log
// that a replay is made from. Nothing here knows what the actions do, or what kind of states
// there are: an engine is generic over the type of its state.
package engine
