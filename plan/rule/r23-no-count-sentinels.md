# R23. No `Count` sentinels (hard)

A constant that stands for "how many values there are" (`ChannelCount`, `KindCount`, `GateCount`, `idEnd`) is not used. It sits among the real values of an enum, so it can be taken for one; it closes the list of values, against [R0](r00-solid.md); and it breaks silently when a value is added after it.

Where a table or an array is sized by the values of an enum, use a table that is open to extension instead: a map keyed by the value, or an entry that declares itself. A list of values that has to be walked is kept where the values are defined.

When a count really is needed, ask the owner first. Then it is named so that it cannot be mistaken for a value, and it carries a comment that says plainly that it is a count and not a value that can be used.
