// Package checkpoint writes the checkpoints that the engines of a cluster take, to files, and reads
// them back. Each engine has its own checkpoints, one file for each tick it took one at, in the
// directory of the engine next to its event log (see replay.EngineDir); no file of one engine
// depends on another. What a checkpoint holds is the business of package engine: here it is bytes.
package checkpoint
