// Package sim is what a game that runs in ticks is built from, whatever the game is: the states of
// its engines, the actions that change a state, the commands that come into an engine from outside,
// and the messages that go from one engine to another.
//
// Everything here is an interface or a mechanism over one: a State is anything that has an ID, a
// type and a hash; an Action[S] is code that changes a state of type S and never leaves it; a
// Command[S] is what comes into an engine and translates itself into actions of S; a Message is a
// list of actions that one engine sends to another. The engine that runs them is in package
// engine, and the game that gives them meaning is a layer above, which imports this package and is
// never imported by it (R1, R2).
package sim
