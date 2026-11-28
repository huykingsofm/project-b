package zonelayer_test

import (
	"testing"

	"github.com/huykingsofm/project-b/server/game/sim/state"
	"github.com/huykingsofm/project-b/server/game/sim/state/zonelayer"
)

// aPopulatedZoneLayer holds 150 characters, each with an item on the ground by it.
func aPopulatedZoneLayer() (*zonelayer.State, state.EntityID, state.EntityID) {
	st := newState()
	var body, item state.EntityID
	for i := range 150 {
		at := state.Vec{X: int32(1000 + 40*i), Y: 2500}
		character := newCharacter(state.CharacterID(i+1), "Binh", 100, 250, 256)
		body = st.AddCharacterEntity(character, at)
		item = st.AddItemEntity(state.ItemID(i%9+1), 3, at, 500)
	}
	return st, body, item
}

// BenchmarkMethods is each method of the state that an action calls, on a zonelayer of 150
// characters. A method that makes something is measured with the one that takes it away, in the
// same run, so that the state is the same at each; the name says both.
func BenchmarkMethods(b *testing.B) {
	st, body, item := aPopulatedZoneLayer()
	at := state.Vec{X: 3000, Y: 2500}
	extra := newCharacter(1000, "Binh", 100, 250, 256)
	text := zonelayer.NewChatMessage(1, "hello", 10)

	b.Run("CharacterByID", func(b *testing.B) {
		for b.Loop() {
			st.CharacterByID(75)
		}
	})
	b.Run("EntityByID", func(b *testing.B) {
		for b.Loop() {
			st.EntityByID(body)
		}
	})
	b.Run("ItemEntityByID", func(b *testing.B) {
		for b.Loop() {
			st.ItemEntityByID(item)
		}
	})
	b.Run("CharacterEntityByCharacterID", func(b *testing.B) {
		for b.Loop() {
			st.CharacterEntityByCharacterID(75)
		}
	})
	b.Run("CooldownUntil", func(b *testing.B) {
		for b.Loop() {
			st.CooldownUntil(75, 1)
		}
	})
	b.Run("SetCooldown", func(b *testing.B) {
		for b.Loop() {
			st.SetCooldown(75, 1, 40)
		}
	})
	b.Run("EntityIDsOfKind", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.EntityIDsOfKind(state.ItemKindID)
		}
	})
	b.Run("AddCharacterEntity and RemoveCharacterEntity", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.AddCharacterEntity(extra, at)
			st.RemoveCharacterEntity(extra.CharacterID())
			st.TakeEvents()
		}
	})
	b.Run("AddItemEntity and RemoveItemEntity", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			id := st.AddItemEntity(7, 1, at, 0)
			st.RemoveItemEntity(id)
			st.TakeEvents()
		}
	})
	b.Run("DamageEntity", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.DamageEntity(body, 0)
			st.TakeEvents()
		}
	})
	b.Run("HealAllEntities", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.HealAllEntities(1)
			st.TakeEvents()
		}
	})
	b.Run("SendMessage", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st.SendMessage(text)
			st.TakeEvents()
		}
	})
}
