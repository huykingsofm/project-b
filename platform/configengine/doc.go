// Package configengine is the engine that owns the configuration. Its state holds the overrides of
// the variables, and the snapshot made from them, as the one owner of that data (R28). A config
// service tells it what changed with an UpdateConfigCommand; the other engines read the snapshot
// from the copy it publishes, through their stable view, so the log of each engine says which
// copy it read at every tick, and the log of this engine says every change.
package configengine
