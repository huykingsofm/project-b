# R12. Every piece of data has exactly one owning engine (hard)

A piece of data belongs to exactly one engine, and only the goroutine of that engine ever changes it. No two engines modify the same data, and no data is modified by an action of another engine.

The owner is decided by the scope the data belongs to: the fighting stats of a character belong to its zonelayer, while its items belong to the world. Another engine reads a piece of data only through the published `Clone`, and asks for a change by sending a message to the owner.

Moving data from one engine to another follows two orders that never swap:

- **Remove first, then send.** The engine that gives the data up removes it from its own state in the action, and only then sends the message that hands it over. It never sends first and removes later, because the data would exist in both engines.
- **Save after the tick ends.** Data is written to a checkpoint or a store only after the tick that changed it has ended, never in the middle of a tick.

If a hand-over fails, the data may be lost but it is never duplicated: an item that is lost can be restored from the log, a duplicated item cannot be told apart.
