# R7. No action is treated specially (hard)

No action (Move, Attack, Chat, ...) has a field, a pattern, a part of the protocol or a code path of its own. When a need first shows up with one action, name the general concept and put the need there: a **gate** per character with `until`, `seq` and `done`, used by every action; each action decides its own rule for a busy gate through that one mechanism (it may wait its turn, wait up to a buffer, or be refused). When an action has to wait, it sets itself at the tick it knows it can run, and does not try again at every tick. An action is treated differently only when the owner proposes it.

Why: the effect system depends on every action going through the same pipeline and the same mechanisms, so that an effect can change any action.
