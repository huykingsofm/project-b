# R22. Design for what is coming, not only for what is used today (hard)

When building a mechanism (a type, a container, an interface, a kind of config value, a part of the protocol), design it for the whole set of uses that the design documents and the plan foresee, not for the smallest set that the code written so far happens to need. Before choosing the shape of a mechanism, read the design and the plan to see what will use it, and write the foreseen uses in the note of its exemplar ([R6](r06-pattern-exemplar.md)).

The shape of a mechanism must not be fixed by today's use. Adding the next kind or the next use is a matter of adding, not of changing what exists. For example, a configuration that holds every kind of value the design needs (integers, floating-point numbers, durations, lists, strings, ...) and that takes a new kind by adding one declaration function, not an integer-only configuration that was enough for the code of the day.

This is not building features early. The feature that uses a mechanism waits until it is needed, and an opt-in optimization waits as [R5](r05-essentials-built-now.md) says; only the shape of the mechanism is complete from the start. When it is not clear what the foreseeable uses are, **ask** (see [ways of working](ways-of-working.md)) before fixing the shape; do not choose the smallest one and leave it.
