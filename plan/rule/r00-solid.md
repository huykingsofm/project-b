# R0. SOLID, above all open for extension (hard)

Every design follows the SOLID principles. This rule comes before all the others, and when it conflicts with another rule, the other rule is read in its light.

- **Open for extension, closed for modification (above all).** A new kind, a new use or a new provider is added by writing new code, not by changing code that exists and works. A mechanism therefore takes what varies as something it is given (a function, an interface, an entry that declares itself) and not as a branch inside it. A `switch` or a chain of conditionals over kinds, or a central list that must be edited for every new kind, is a sign that the design is closed to extension and has to be reshaped. Ask of every mechanism: what is the next thing that will be added, and can it be added without touching what is here? (This goes with [R22](r22-design-for-the-future.md): the shape of the mechanism is made for the uses to come.)
- **Single responsibility.** A type, a function and a file have one reason to change ([R12](r12-organization.md)).
- **Liskov substitution.** Every implementation of an interface keeps the whole contract of the interface, so that any of them can stand in for another without the caller knowing which it is.
- **Interface segregation.** An interface is small and has only what its caller needs ([R1](r01-layer-dependency.md)).
- **Dependency inversion.** Code depends on interfaces that the caller defines, not on concrete providers ([R1](r01-layer-dependency.md)).

- **Refactor boldly.** Whatever stands in the way of a better design may be changed as much as needed: renamed, moved, split, deleted, or replaced wholly by a new design, with the old one removed and not kept beside it ([R10](r10-writing.md), [R11](r11-refactor.md)). Only a replacement that changes something the owner decided, or that the owner could not foresee from the plan, is proposed to the owner first; any other is done and reported afterwards.

Where the owner's own design fixes a central place on purpose (the action IDs are constants of one file), that is the owner's decision and stays; it is not a reason to add other central places.
