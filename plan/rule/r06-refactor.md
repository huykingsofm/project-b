# R6. Change the code: refactor at once, replace in place, look first (hard)

Refactor as soon as it is possible, in the same change or in a pull request right after; do not leave it for later. When a refactor gives a better design, do the refactor. Do not work around the problem, and do not add a light layer on top of a design that should change.

**How far.** Whatever stands in the way of a better design may be changed as much as needed: renamed, moved, split, deleted, or replaced wholly by a new design, with the old one removed and not kept beside it (see below). Only a replacement that changes something the owner decided, or that the owner could not foresee from the plan, is proposed to the owner first; any other is done and reported afterwards.

## Replace in place

A replacement is made in place and the old text is deleted: no old copies, no "superseded" notes, no compatibility shims, no dead code. Edited text reads as if it had always been so (no "no longer", "instead of", "changed from"), and every other mention of the replaced thing is updated in the same change. Scans for leftovers are semantic reviews of docs, code, tests and names, not keyword searches. Tests are code: a replaced or deleted thing takes its tests with it, and a test is written for what exists, never because something was removed.

## Look before you write

Before a function, a type, a constant, a file or a test helper is written, the project is searched for one that does the same, or nearly the same. When there is one, it is used. When one is near, it is changed so that both uses are served and the second is not written. When two that do the same already exist, they are merged into one, in the same change.

- **Where to look:** the package, the layers below it (`platform`, `shared`), the tests and their helpers, and the docs ([R14](r14-docs-are-reference.md)); by name, by what it takes and returns, and by shape (a struct with the same fields, a map with the same key, a loop that does the same walk).
- **Same shape is the same thing:** two types with the same fields under two names are one type with one name, and the name says what it is for everywhere it is used.
- **A new one is written only when none can be made to serve,** and the change says which was looked at and why it does not fit.
- **Allowed:** an interface that each layer defines for itself ([R1](r01-layer-dependency.md)), and a copy that has to stay apart so that one can change without the other, with a comment that says so.
- **Not allowed:** a second constructor, helper, type or test fixture for what already exists, because the first was not looked for; and leaving two of a kind in the code once they are seen.

Why: a duplicate is fixed in one place and left wrong in the other, and a reader cannot tell whether two names mean two things. (When the replay and the recovery were put in one package, `copyKey`, `copyRead` and `replayNeed` were found to be one type three times.)
