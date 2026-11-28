# R14. Documents of the code are reference, written as structure (hard)

The documents under `docs/` are looked up as much as they are read, so each is a set of structured paragraphs.

- **Structure first.** A fact is a list item, a key and its value, a table row or a numbered step. A sentence of explanation appears only when the structure cannot say the thing and a reader would otherwise get it wrong; "why it exists" is not such a reason.
- **One shape for one kind of thing.** Every action is documented with the same keys in the same order, every entity the same way, so that a reader finds a fact by its key. A key that does not apply to a thing is left out, not filled with "none".
- **Nothing the code or a rule already says.** A file name, an ID type or a name that follows from a rule (R11) is stated once, in the opening of the document, and not repeated for each thing. Who authored an action (`Actor()`) is not documented.
- **One home for a fact.** Entities are in `docs/entity/`, actions in `docs/action/`, and a document links to the other instead of copying it. When a thing changes, its document changes in the same change (R6).
- **Examples are concrete and short**: numbers and ticks, one line per case.
