# R11. Actions: names, files and IDs (hard)

Every action belongs to exactly one scope: the world, a zone or a zonelayer. (The word for them is **scope**; `level` clashes with the level of a character.) All the actions are in the one package `action`, and what ties each one to its scope is a rule:

- **The type is the scope, then the words of the name, then `Action`:** `ZoneLayerCharacterMoveAction`, `ZoneChatSendAction`, `WorldChatPrivateSendAction`.
- **The file is the scope, then the same words, then `_action.go`:** `zonelayer_character_move_action.go`, `zone_chat_send_action.go`, `world_chat_private_send_action.go`. One action to a file.
- **The last word of the name is a verb:** what the action does, as in `ZoneLayerCharacterMoveAction`, `ZoneLayerCharacterNormalAttackTriggerAction` (a character triggers its normal attack) and `ZoneLayerCharacterShieldGrantAction` (a character is granted a shield). The verb may be a phrasal verb that ends in a preposition (`ZoneLayerCharacterExitToAction`). A name that ends in a noun (`...NormalAttackAction`, `...ShieldAction`) is given a verb. A command takes the name of its action, so it has the same verb.
- **The words of the name go from the general to the specific,** so that related actions share the beginning of their names and stand next to each other in a listing of files: `zonelayer_character_attack_action.go`, `zonelayer_character_damage_action.go`, `zonelayer_character_enter_action.go`, `zonelayer_character_exit_to_action.go`, `zonelayer_character_move_action.go`. A new action takes its place among the actions it is related to; when related actions are not next to each other, they are renamed in the same change ([R6](r06-refactor.md)).
- **It implements the action of its scope:** `sim.Action[*zonelayerstate.State]`, `sim.Action[*zonestate.State]` or `sim.Action[*worldstate.State]`, and is registered with the type of state it is for (`RegisterAction[S]` in `register.go`), so an action cannot be given to an engine of another state, nor read back as one. Its method is `Apply`, the same for every scope.
- **An action that has something to settle when it is cancelled** implements `sim.Cancellable[S]`, which embeds `sim.Action[S]` and adds the method `ApplyCancel`. It is an action in every way: it has an ID and a priority.
- **Its ID is a number of the register:** a constant of type `sim.ActionID` (a `uint32`) named `<Scope><Name>ActionID` in `id.go`, the register of the kinds of action, that the action's `ID()` returns. The numbers count up from 1; a new action takes the next one at the end of the list, and a number is never changed or reused, because the event log writes it, so a log needs no table to map an ID. A cooldown key is made the same way: a constant of type `CooldownKey` in `cooldown_key.go`. A test checks that every action has an ID of its own and every ID an action.
- **An action's author** (the actor that performs it: a character, a player) is given by the method `Actor()` of `sim.BaseAction`, which every action has: an action that nobody performs (one the game does by itself) returns `nil`. The author is compared with `==`, so it is of a type that can be compared.

## Commands

A command is named, filed and numbered the same way:

- **It is named like the action it translates to,** with `Command` in place of `Action`: `ZoneLayerCharacterMoveCommand` for `ZoneLayerCharacterMoveAction`.
- **Its file is `<scope>_<words>_command.go`, one command to a file:** `zonelayer_character_move_command.go`. The file holds the type, `ID()`, the translation into actions, `Encode` and the function `decode<Type>` that reads what `Encode` wrote.
- **Its ID is a number of the register `command/id.go`,** a constant `<Type>ID` of type `ID`, counted from 1, added at the end and never changed, because the event log writes it. The function that reads the command is listed in the table `decoders` by that ID.
- **It implements `Command`** (an `ID()` and an `Encode`) and the interface of its scope, `ZoneLayerCommand`, `ZoneCommand` or `WorldCommand`.

