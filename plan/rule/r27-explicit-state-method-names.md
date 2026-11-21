# R27. State methods have explicit names

The name of a method of a state says plainly what it does and to what, even when that makes it long. It does not have to read well, it has to be unambiguous: `SendMessage`, not `Say`; `AddCharacterEntity`, not `Attach`; `MoveEntity`, not `Step`; `DamageEntity`, not `Hurt`.

A state method that reads or writes something of one kind of thing names that thing: `CharacterByID`, `CooldownUntil`, `SendPrivateMessage`.
