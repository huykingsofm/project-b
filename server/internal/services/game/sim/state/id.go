// Package state holds what the states of the three scopes (world, zone, zonelayer) have in common:
// the IDs and the plain values that more than one of them uses. The state of each scope is in a
// package of its own below this one, and its events in a package of its own below that.
package state

// WorldID names a world, ZoneID a zone and ZoneLayerID a zonelayer. A zonelayer ID is unique in the
// world, so a message can be addressed to a zonelayer by it alone.
type (
	WorldID     uint32
	ZoneID      uint32
	ZoneLayerID uint32
)

// CharacterID names a character: a figure in the game. A character is not always played by a
// person. PlayerID names a player: the person who plays. A character may be linked to a player
// or not.
type (
	CharacterID uint64
	PlayerID    uint64
)

// EntityID names the body a character has in a zonelayer, which is new each time the character
// enters a zonelayer.
type EntityID uint64

// ItemID names a kind of item: what a dropped item is, and what an inventory holds.
type ItemID uint32
