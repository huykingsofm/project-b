# R4. What the design makes essential is built now, and its shape is made for what is coming (hard)

If the design says that every engine, every message or every level has something (a writer, an inbox, a header, a field), define it and build it in the change that finds the gap, with a test of what it does, and write it into the design documents and the plan. It does not wait until something uses it. Only **optimization** waits: an opt-in way to make something that already works faster is deferred and recorded as pending, and benchmarks are still run and recorded. If this rule conflicts with another, the other is changed to match.

## The shape is made for what is coming

When building a mechanism (a type, a container, an interface, a kind of config value, a part of the protocol), design it for the whole set of uses that the design documents and the plan foresee, not for the smallest set that the code written so far happens to need. Before choosing the shape of a mechanism, read the design and the plan to see what will use it, and write the foreseen uses in the note of its exemplar ([R5](r05-pattern-exemplar.md)).

The shape of a mechanism must not be fixed by today's use. Adding the next kind or the next use is a matter of adding, not of changing what exists. For example, a configuration that holds every kind of value the design needs (integers, floating-point numbers, durations, lists, strings, ...) and that takes a new kind by adding one declaration function, not an integer-only configuration that was enough for the code of the day.

This is not building features early. The feature that uses a mechanism waits until it is needed, and an opt-in optimization waits as the first paragraph says; only the shape of the mechanism is complete from the start. When it is not clear what the foreseeable uses are, **ask** (see [ways of working](ways-of-working.md)) before fixing the shape; do not choose the smallest one and leave it.
